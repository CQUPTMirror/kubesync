package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/internal"
	"github.com/gin-gonic/gin"
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
	namespace  string
}

func contextErrorLogger(c *gin.Context) {
	errs := c.Errors.ByType(gin.ErrorTypeAny)
	if len(errs) > 0 {
		for _, err := range errs {
			runLog.Error(err, fmt.Sprintf(`"in request "%s %s: %s"`, c.Request.Method, c.Request.URL.Path, err.Error()))
		}
	}
	// pass on to the next middleware in chain
	c.Next()
}

func GetTUNASyncManager(config *rest.Config, options Options) (*Manager, error) {
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		return nil, errors.New("can't get namespace")
	}
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

	cl, err := client.NewDelegatingClient(client.NewDelegatingClientInput{CacheReader: cc, Client: c})
	if err != nil {
		return nil, err
	}

	s := &Manager{
		config:    config,
		client:    cl,
		internal:  context.Background(),
		cache:     cc,
		address:   options.Address,
		namespace: namespace,
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
	s.engine.GET("/jobs", s.listJob)

	// create job
	s.engine.POST("/job", s.createJob)

	// mirrorID should be valid in this route group
	mirrorValidateGroup := s.engine.Group("/jobs/:id")
	{
		// delete specified mirror
		mirrorValidateGroup.DELETE("", s.deleteJob)
		// get job detail
		mirrorValidateGroup.GET("", s.getJob)
		mirrorValidateGroup.GET("config", s.getJobConfig)
		mirrorValidateGroup.GET("log", s.getJobLatestLog)
		// mirror online
		mirrorValidateGroup.POST("", s.registerMirror)
		// post job status
		mirrorValidateGroup.PATCH("", s.updateJob)
		mirrorValidateGroup.POST("size", s.updateMirrorSize) // TODO: kubelet_volume_stats_used_bytes method to get size
		mirrorValidateGroup.POST("schedule", s.updateSchedule)
		mirrorValidateGroup.POST("disable", s.disableJob)
		mirrorValidateGroup.DELETE("pod", s.restartPod)
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
func (m *Manager) Run(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:         m.address,
		Handler:      m.engine,
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

func (m *Manager) GetJobRaw(c *gin.Context, mirrorID string) (*v1beta1.Job, error) {
	job := new(v1beta1.Job)
	err := m.client.Get(c.Request.Context(), client.ObjectKey{Namespace: m.namespace, Name: mirrorID}, job)
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

func (m *Manager) GetJob(c *gin.Context, mirrorID string) (w internal.MirrorStatus, err error) {
	job, err := m.GetJobRaw(c, mirrorID)
	w = internal.MirrorStatus{ID: mirrorID, JobStatus: job.Status}
	return
}

func (m *Manager) UpdateJobStatus(c *gin.Context, w internal.MirrorStatus) error {
	job, err := m.GetJobRaw(c, w.ID)
	if err != nil {
		return err
	}
	job.Status = w.JobStatus
	job.Status.LastOnline = time.Now().Unix()
	err = m.client.Status().Update(c.Request.Context(), job)
	return err
}

func (m *Manager) createJob(c *gin.Context) {
	// ctx context.Context, c internal.MirrorConfig) error
	//job := &v1beta1.Job{
	//	ObjectMeta: metav1.ObjectMeta{Name: c.ID, Namespace: m.namespace},
	//	Spec:       c.JobSpec,
	//}
	//return m.client.Create(ctx, job)
}

// listJob respond with all jobs of specified mirrors
func (m *Manager) listJob(c *gin.Context) {
	var ws []internal.MirrorStatus

	m.rwmu.RLock()
	jobs := new(v1beta1.JobList)
	err := m.client.List(c.Request.Context(), jobs)
	m.rwmu.RUnlock()

	for _, v := range jobs.Items {
		w := internal.MirrorStatus{ID: v.Name, JobStatus: v.Status}
		ws = append(ws, w)
	}

	if err != nil {
		err := fmt.Errorf("failed to list mirrors: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (m *Manager) getJob(c *gin.Context) {
	mirrorID := c.Param("id")
	var status internal.MirrorStatus

	m.rwmu.Lock()
	status, err := m.GetJob(c, mirrorID)
	m.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to get job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func (m *Manager) getJobConfig(c *gin.Context) {
	mirrorID := c.Param("id")
	var config internal.MirrorConfig

	m.rwmu.Lock()
	job, err := m.GetJobRaw(c, mirrorID)
	config = internal.MirrorConfig{ID: mirrorID, JobSpec: job.Spec}
	m.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to get job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, config)
}

func (m *Manager) getJobLatestLog(c *gin.Context) {
	mirrorID := c.Param("id")
	if mirrorID == "" {
		// TODO: decide which mirror should do this mirror when MirrorID is null string
		runLog.Info("handleClientCmd case mirrorID == \" \" not implemented yet")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	runLog.Info(fmt.Sprintf("Geting log from <%s>", mirrorID))

	if m.httpClient == nil {
		m.httpClient = &http.Client{
			Transport: &http.Transport{MaxIdleConnsPerHost: 20},
			Timeout:   5 * time.Second,
		}
	}
	resp, err := m.httpClient.Get(fmt.Sprintf("http://%s:6000/log", mirrorID))

	if err != nil {
		err := fmt.Errorf("get log from mirror %s fail: %s", mirrorID, err.Error())
		c.Error(err)
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)

	c.String(http.StatusOK, string(bodyText))
}

// deleteJob deletes one job by id
func (m *Manager) deleteJob(c *gin.Context) {
	mirrorID := c.Param("id")

	m.rwmu.Lock()
	job, err := m.GetJobRaw(c, mirrorID)
	m.rwmu.Unlock()

	if err != nil {
		return
	}
	err = m.client.Delete(c.Request.Context(), job)
	if err != nil {
		err := fmt.Errorf("failed to delete mirror: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	runLog.Info(fmt.Sprintf("Mirror <%s> deleted", mirrorID))
	c.JSON(http.StatusOK, gin.H{_infoKey: "deleted"})
}

// registerMirror register a newly-online mirror
func (m *Manager) registerMirror(c *gin.Context) {
	mirrorID := c.Param("id")
	m.rwmu.Lock()
	status, err := m.GetJob(c, mirrorID)
	m.rwmu.Unlock()

	if err != nil {
		runLog.Error(err, fmt.Sprintf("Failed to get status of job %s: %s", mirrorID, err.Error()))
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	status.LastOnline = time.Now().Unix()
	status.LastRegister = time.Now().Unix()

	m.rwmu.Lock()
	err = m.UpdateJobStatus(c, status)
	m.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to register mirror %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	runLog.Info(fmt.Sprintf("Mirror <%s> registered", mirrorID))
	c.JSON(http.StatusOK, status)
}

func (m *Manager) returnErrJSON(c *gin.Context, code int, err error) {
	c.JSON(code, gin.H{
		_errorKey: err.Error(),
	})
}

func (m *Manager) updateSchedule(c *gin.Context) {
	mirrorID := c.Param("id")
	type empty struct{}
	var schedule internal.MirrorSchedule
	c.BindJSON(&schedule)

	m.rwmu.Lock()
	curStatus, err := m.GetJob(c, mirrorID)
	m.rwmu.Unlock()

	if err != nil {
		runLog.Error(err, fmt.Sprintf("failed to get job %s: %s", mirrorID, err.Error()))
		c.JSON(http.StatusOK, empty{})
	}

	if curStatus.Scheduled == schedule.NextSchedule {
		// no changes, skip update
		c.JSON(http.StatusOK, empty{})
	}

	curStatus.Scheduled = schedule.NextSchedule
	m.rwmu.Lock()
	err = m.UpdateJobStatus(c, curStatus)
	m.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to update job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, empty{})
}

func (m *Manager) updateJob(c *gin.Context) {
	mirrorID := c.Param("id")
	var status internal.MirrorStatus
	c.BindJSON(&status)

	m.rwmu.Lock()
	curStatus, err := m.GetJob(c, mirrorID)
	m.rwmu.Unlock()

	curTime := time.Now().Unix()

	status.LastOnline = curTime
	status.LastRegister = curStatus.LastRegister

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
		runLog.Info(fmt.Sprintf("Job [%s] starts syncing", status.ID))
	default:
		runLog.Info(fmt.Sprintf("Job [%s] %s", status.ID, status.Status))
	}

	m.rwmu.Lock()
	err = m.UpdateJobStatus(c, status)
	m.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to update job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func (m *Manager) updateMirrorSize(c *gin.Context) {
	mirrorID := c.Param("id")
	type SizeMsg struct {
		Size string `json:"size"`
	}
	var msg SizeMsg
	c.BindJSON(&msg)

	m.rwmu.Lock()
	status, err := m.GetJob(c, mirrorID)
	m.rwmu.Unlock()

	if err != nil {
		runLog.Error(err, fmt.Sprintf("Failed to get status of job %s: %s", mirrorID, err.Error()))
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	// Only message with meaningful size updates the mirror size
	if len(msg.Size) > 0 || msg.Size != "unknown" {
		status.Size = msg.Size
	}

	runLog.Info(fmt.Sprintf("Mirror size of [%s]: %s", status.ID, status.Size))

	m.rwmu.Lock()
	err = m.UpdateJobStatus(c, status)
	m.rwmu.Unlock()

	if err != nil {
		err := fmt.Errorf("failed to update job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func (m *Manager) disableJob(c *gin.Context) {
	mirrorID := c.Param("id")

	m.rwmu.Lock()
	curStat, err := m.GetJob(c, mirrorID)
	m.rwmu.Unlock()

	if err != nil {
		return
	}

	curStat.Status = v1beta1.Disabled
	m.rwmu.Lock()
	m.UpdateJobStatus(c, curStat)
	m.rwmu.Unlock()

	// err = s.client.Delete(c.Request.Context(), job)
	if err != nil {
		err := fmt.Errorf("failed to delete mirror: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	runLog.Info(fmt.Sprintf("Mirror <%s> deleted", mirrorID))
	c.JSON(http.StatusOK, gin.H{_infoKey: "deleted"})
}

func (m *Manager) restartPod(c *gin.Context) {
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
	return client.Post(fmt.Sprintf("http://%s:6000", mirrorID), "application/json; charset=utf-8", b)
}

func (m *Manager) handleClientCmd(c *gin.Context) {
	mirrorID := c.Param("id")
	var clientCmd internal.ClientCmd
	c.BindJSON(&clientCmd)
	if mirrorID == "" {
		// TODO: decide which mirror should do this mirror when MirrorID is null string
		runLog.Info("handleClientCmd case mirrorID == \" \" not implemented yet")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	m.rwmu.Lock()
	curStat, err := m.GetJob(c, mirrorID)
	m.rwmu.Unlock()

	changed := false
	switch clientCmd.Cmd {
	case internal.CmdStop:
		curStat.Status = v1beta1.Paused
		changed = true
	}
	if changed {
		m.rwmu.Lock()
		m.UpdateJobStatus(c, curStat)
		m.rwmu.Unlock()
	}

	runLog.Info(fmt.Sprintf("Posting command '%s' to <%s>", clientCmd.Cmd, mirrorID))
	// post command to mirror
	_, err = PostJSON(mirrorID, clientCmd, m.httpClient)
	if err != nil {
		err := fmt.Errorf("post command to mirror %s fail: %s", mirrorID, err.Error())
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	// TODO: check response for success
	c.JSON(http.StatusOK, gin.H{_infoKey: "successfully send command to mirror " + mirrorID})
}
