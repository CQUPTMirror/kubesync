package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/internal"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	kubelog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	_errorKey = "error"
	_infoKey  = "message"
)

var (
	defaultRetryPeriod = 2 * time.Second
	runLog             = kubelog.Log.WithName("kubesync").WithName("run")
)

type Options struct {
	Scheme  *runtime.Scheme
	Address string
}

type Manager struct {
	config     *rest.Config
	engine     *gin.Engine
	httpClient *http.Client
	client     client.Client
	started    bool
	internal   context.Context
	cache      cache.Cache
	address    string
	rwmu       sync.RWMutex
}

func contextErrorLogger(c *gin.Context) {
	errs := c.Errors.ByType(gin.ErrorTypeAny)
	if len(errs) > 0 {
		for _, err := range errs {
			runLog.Error(err, `"in request "%s %s: %s"`,
				c.Request.Method, c.Request.URL.Path,
				err.Error())
		}
	}
	// pass on to the next middleware in chain
	c.Next()
}

func GetTUNASyncManager(config *rest.Config, options Options) (*Manager, error) {
	mapper, err := apiutil.NewDynamicRESTMapper(config)
	if err != nil {
		return nil, err
	}

	cc, err := cache.New(config, cache.Options{
		Scheme: options.Scheme,
		Mapper: mapper,
		Resync: &defaultRetryPeriod,
	})
	if err != nil {
		return nil, err
	}

	c, err := client.New(config, client.Options{Scheme: options.Scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	client, err := client.NewDelegatingClient(client.NewDelegatingClientInput{CacheReader: cc, Client: c})
	if err != nil {
		return nil, err
	}

	s := &Manager{
		config:   config,
		client:   client,
		internal: context.Background(),
		cache:    cc,
		address:  options.Address,
	}

	gin.SetMode(gin.ReleaseMode)

	s.engine = gin.New()
	s.engine.Use(gin.Recovery())

	// common log middleware
	s.engine.Use(contextErrorLogger)

	s.engine.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{_infoKey: "pong"})
	})

	// list jobs, status page
	s.engine.GET("/jobs", s.listAllJobs)

	s.engine.GET("/jobs/:ns", s.listNamespacedJobs)

	// mirror online
	s.engine.POST("/jobs/:ns", s.registerMirror)

	// mirrorID should be valid in this route group
	mirrorValidateGroup := s.engine.Group("/jobs/:ns/:id")
	{
		// delete specified mirror
		mirrorValidateGroup.DELETE("", s.deleteJob)
		// get job detail
		mirrorValidateGroup.GET("", s.getJob)
		// post job status
		mirrorValidateGroup.POST("", s.updateJob)
		mirrorValidateGroup.POST("size", s.updateMirrorSize)
		mirrorValidateGroup.POST("schedule", s.updateSchedule)
		mirrorValidateGroup.POST("disable", s.disableJob)
		// for tunasynctl to post commands
		mirrorValidateGroup.POST("cmd", s.handleClientCmd)
	}

	return s, nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.waitForCache()

	runLog.Info("Tunasync manager server is starting to listen " + m.address)

	go func() {
		if err := m.Run(m.internal); err != nil {
			panic(err)
		}
	}()
	select {
	case <-ctx.Done():
		return nil
	}
}

func (m *Manager) waitForCache() {
	if m.started {
		return
	}

	go func() {
		if err := m.cache.Start(m.internal); err != nil {
			panic(err)
		}
	}()

	// Wait for the caches to sync.
	m.cache.WaitForCacheSync(m.internal)
	m.started = true
}

// Run runs the manager server forever
func (s *Manager) Run(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:         s.address,
		Handler:      s.engine,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		err := httpServer.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
	select {
	case <-ctx.Done():
		runLog.Info("Shutting down apiserver")
		return httpServer.Shutdown(context.Background())
	}
}

func (m *Manager) GetJobRaw(c *gin.Context, namespace, mirrorID string) (*v1beta1.Job, error) {
	job := new(v1beta1.Job)
	err := m.client.Get(c.Request.Context(), client.ObjectKey{Namespace: namespace, Name: mirrorID}, job)
	if err != nil {
		err := fmt.Errorf("failed to get mirror: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return nil, err
	}
	return job, err
}

func (m *Manager) GetJob(c *gin.Context, namespace, mirrorID string) (w internal.MirrorStatus, err error) {
	job, err := m.GetJobRaw(c, namespace, mirrorID)
	w = internal.MirrorStatus{MirrorBase: internal.MirrorBase{ID: mirrorID, Namespace: namespace}, JobStatus: job.Status}
	return
}

func (m *Manager) UpdateJobStatus(c *gin.Context, w internal.MirrorStatus) error {
	job, err := m.GetJobRaw(c, w.Namespace, w.ID)
	if err != nil {
		return err
	}
	job.Status = w.JobStatus
	job.Status.LastOnline = time.Now().Unix()
	err = m.client.Update(c.Request.Context(), job)
	return err
}

func (m *Manager) CreateJob(ctx context.Context, c internal.MirrorConfig) error {
	job := &v1beta1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: c.ID, Namespace: c.Namespace},
		Spec:       c.JobSpec,
	}
	return m.client.Create(ctx, job)
}

// listAllJobs respond with all jobs of specified mirrors
func (s *Manager) listAllJobs(c *gin.Context) {
	var ws []internal.MirrorStatus

	s.rwmu.RLock()
	jobs := new(v1beta1.JobList)
	err := s.client.List(c.Request.Context(), jobs)
	s.rwmu.RUnlock()

	for _, v := range jobs.Items {
		w := internal.MirrorStatus{MirrorBase: internal.MirrorBase{ID: v.Name, Namespace: v.Namespace}, JobStatus: v.Status}
		ws = append(ws, w)
	}

	if err != nil {
		err := fmt.Errorf("failed to list mirrors: %s",
			err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (s *Manager) listNamespacedJobs(c *gin.Context) {
	namespace := c.Param("ns")
	var ws []internal.MirrorStatus

	s.rwmu.RLock()
	jobs := new(v1beta1.JobList)
	err := s.client.List(c.Request.Context(), jobs, &client.ListOptions{Namespace: namespace})
	s.rwmu.RUnlock()

	for _, v := range jobs.Items {
		w := internal.MirrorStatus{MirrorBase: internal.MirrorBase{ID: v.Name, Namespace: v.Namespace}, JobStatus: v.Status}
		ws = append(ws, w)
	}

	if err != nil {
		err := fmt.Errorf("failed to list mirrors: %s",
			err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (s *Manager) getJob(c *gin.Context) {
	namespace := c.Param("ns")
	mirrorID := c.Param("id")
	var status internal.MirrorStatus
	c.BindJSON(&status)

	s.rwmu.Lock()
	status, err := s.GetJob(c, namespace, mirrorID)
	s.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to get job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

// deleteJob deletes one job by id
func (s *Manager) deleteJob(c *gin.Context) {
	namespace := c.Param("ns")
	mirrorID := c.Param("id")

	s.rwmu.Lock()
	job, err := s.GetJobRaw(c, namespace, mirrorID)
	s.rwmu.Unlock()

	if err != nil {
		return
	}
	err = s.client.Delete(c.Request.Context(), job)
	if err != nil {
		err := fmt.Errorf("failed to delete mirror: %s",
			err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	runLog.Info("Mirror <%s> deleted", mirrorID)
	c.JSON(http.StatusOK, gin.H{_infoKey: "deleted"})
}

// registerMirror register an newly-online mirror
func (s *Manager) registerMirror(c *gin.Context) {
	var _mirror internal.MirrorStatus
	c.BindJSON(&_mirror)
	_mirror.LastOnline = time.Now().Unix()
	_mirror.LastRegister = time.Now().Unix()
	s.rwmu.Lock()
	err := s.UpdateJobStatus(c, _mirror)
	s.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to register mirror: %s",
			err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	runLog.Info("Mirror <%s> registered", _mirror.ID)
	// create mirrorCmd channel for this mirror
	c.JSON(http.StatusOK, _mirror)
}

func (s *Manager) returnErrJSON(c *gin.Context, code int, err error) {
	c.JSON(code, gin.H{
		_errorKey: err.Error(),
	})
}

func (s *Manager) updateSchedule(c *gin.Context) {
	type empty struct{}

	var schedule internal.MirrorSchedule
	c.BindJSON(&schedule)

	mirrorID := schedule.ID
	namespace := schedule.Namespace
	if len(mirrorID) == 0 || len(namespace) == 0 {
		s.returnErrJSON(
			c, http.StatusBadRequest,
			errors.New("Mirror Name should not be empty"),
		)
	}

	s.rwmu.Lock()
	curStatus, err := s.GetJob(c, namespace, mirrorID)
	s.rwmu.Unlock()

	if err != nil {
		runLog.Error(err, "failed to get job %s: %s",
			mirrorID, err.Error(),
		)
		c.JSON(http.StatusOK, empty{})
	}

	if curStatus.Scheduled == schedule.NextSchedule {
		// no changes, skip update
		c.JSON(http.StatusOK, empty{})
	}

	curStatus.Scheduled = schedule.NextSchedule
	s.rwmu.Lock()
	err = s.UpdateJobStatus(c, curStatus)
	s.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to update job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, empty{})
}

func (s *Manager) updateJob(c *gin.Context) {
	namespace := c.Param("ns")
	mirrorID := c.Param("id")
	var status internal.MirrorStatus
	c.BindJSON(&status)

	s.rwmu.Lock()
	curStatus, err := s.GetJob(c, namespace, mirrorID)
	s.rwmu.Unlock()

	curTime := time.Now().Unix()

	status.LastOnline = curTime

	if status.Status == v1beta1.PreSyncing && curStatus.Status != v1beta1.PreSyncing {
		status.LastStarted = curTime
	} else {
		status.LastStarted = curStatus.LastStarted
	}
	// Only successful syncing needs last_update
	if status.Status == v1beta1.Success {
		status.LastUpdate = curTime
	} else {
		status.LastUpdate = curStatus.LastUpdate
	}
	if status.Status == v1beta1.Success || status.Status == v1beta1.Failed {
		status.LastEnded = curTime
	} else {
		status.LastEnded = curStatus.LastEnded
	}

	// Only message with meaningful size updates the mirror size
	if len(curStatus.Size) > 0 && curStatus.Size != "unknown" {
		if len(status.Size) == 0 || status.Size == "unknown" {
			status.Size = curStatus.Size
		}
	}

	// for logging
	switch status.Status {
	case v1beta1.Syncing:
		runLog.Info("Job [%s] starts syncing", status.ID)
	default:
		runLog.Info("Job [%s] %s", status.ID, status.Status)
	}

	s.rwmu.Lock()
	err = s.UpdateJobStatus(c, status)
	s.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to update job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Manager) updateMirrorSize(c *gin.Context) {
	namespace := c.Param("ns")
	mirrorID := c.Param("id")
	type SizeMsg struct {
		ID   string `json:"id"`
		Size string `json:"size"`
	}
	var msg SizeMsg
	c.BindJSON(&msg)

	mirrorName := msg.ID
	s.rwmu.Lock()
	status, err := s.GetJob(c, namespace, mirrorID)
	s.rwmu.Unlock()

	if err != nil {
		runLog.Error(err,
			"Failed to get status of mirror %s @<%s>: %s",
			mirrorName, mirrorID, err.Error(),
		)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	// Only message with meaningful size updates the mirror size
	if len(msg.Size) > 0 || msg.Size != "unknown" {
		status.Size = msg.Size
	}

	runLog.Info("Mirror size of [%s]: %s", status.ID, status.Size)

	s.rwmu.Lock()
	err = s.UpdateJobStatus(c, status)
	s.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to update job %s of mirror %s: %s",
			mirrorName, mirrorID, err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Manager) disableJob(c *gin.Context) {
	namespace := c.Param("ns")
	mirrorID := c.Param("id")

	s.rwmu.Lock()
	curStat, err := s.GetJob(c, namespace, mirrorID)
	s.rwmu.Unlock()

	if err != nil {
		return
	}

	curStat.Status = v1beta1.Disabled
	s.rwmu.Lock()
	s.UpdateJobStatus(c, curStat)
	s.rwmu.Unlock()

	// err = s.client.Delete(c.Request.Context(), job)
	if err != nil {
		err := fmt.Errorf("failed to delete mirror: %s",
			err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	runLog.Info("Mirror <%s> deleted", mirrorID)
	c.JSON(http.StatusOK, gin.H{_infoKey: "deleted"})
}

// PostJSON posts json object to url
func PostJSON(mirrorID string, obj interface{}, client *http.Client) (*http.Response, error) {
	if client == nil {
		client = &http.Client{
			Transport: &http.Transport{MaxIdleConnsPerHost: 20},
			Timeout:   5 * time.Second,
		}
	}
	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(obj); err != nil {
		return nil, err
	}
	return client.Post("", "application/json; charset=utf-8", b)
}

func (s *Manager) handleClientCmd(c *gin.Context) {
	namespace := c.Param("ns")
	mirrorID := c.Param("id")
	var clientCmd internal.ClientCmd
	c.BindJSON(&clientCmd)
	if mirrorID == "" {
		// TODO: decide which mirror should do this mirror when MirrorID is null string
		runLog.Info("handleClientCmd case mirrorID == \" \" not implemented yet")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	s.rwmu.Lock()
	curStat, err := s.GetJob(c, namespace, mirrorID)
	s.rwmu.Unlock()

	changed := false
	switch clientCmd.Cmd {
	case internal.CmdStop:
		curStat.Status = v1beta1.Paused
		changed = true
	}
	if changed {
		s.rwmu.Lock()
		s.UpdateJobStatus(c, curStat)
		s.rwmu.Unlock()
	}

	runLog.Info("Posting command '%s' to <%s>", clientCmd.Cmd, mirrorID)
	// post command to mirror
	_, err = PostJSON(mirrorID, clientCmd, s.httpClient)
	if err != nil {
		err := fmt.Errorf("post command to mirror %s fail: %s", mirrorID, err.Error())
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	// TODO: check response for success
	c.JSON(http.StatusOK, gin.H{_infoKey: "successfully send command to mirror " + mirrorID})
}
