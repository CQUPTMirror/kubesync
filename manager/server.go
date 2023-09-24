/*
Copyright (C) 2023  CQUPTMirror

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		Scheme:    options.Scheme,
		Mapper:    mapper,
		Resync:    &defaultRetryPeriod,
		Namespace: namespace,
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

	hc := &http.Client{
		Transport: &http.Transport{MaxIdleConnsPerHost: 20},
		Timeout:   5 * time.Second,
	}

	s := &Manager{
		config:     config,
		httpClient: hc,
		client:     cl,
		internal:   context.Background(),
		cache:      cc,
		address:    options.Address,
		namespace:  namespace,
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
	s.engine.GET("/mirrors", s.listJob)

	// mirrorID should be valid in this route group
	mirrorValidateGroup := s.engine.Group("/job/:id")
	{
		// delete specified mirror
		mirrorValidateGroup.DELETE("", s.deleteJob)
		// get job detail
		mirrorValidateGroup.GET("", s.getJob)
		mirrorValidateGroup.GET("config", s.getJobConfig)
		mirrorValidateGroup.GET("log", s.getJobLatestLog)
		// create or patch job
		mirrorValidateGroup.PATCH("", s.createJob)
		// mirror online
		mirrorValidateGroup.PUT("", s.registerMirror)
		// post job status
		mirrorValidateGroup.POST("", s.updateJob)
		mirrorValidateGroup.POST("size", s.updateMirrorSize)
		mirrorValidateGroup.POST("schedule", s.updateSchedule)
		mirrorValidateGroup.POST("enable", s.enableJob)
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

func (m *Manager) GetJob(c *gin.Context, mirrorID string) (*v1beta1.Job, error) {
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

func handleMerge(c *gin.Context, oJobSpec, jobSpec *map[string]map[string]interface{}) (merged *v1beta1.JobSpec) {
	if val, ok := (*jobSpec)["config"]; ok {
		for k, v := range val {
			(*oJobSpec)["config"][k] = v
		}
	}
	if val, ok := (*jobSpec)["deploy"]; ok {
		for k, v := range val {
			(*oJobSpec)["deploy"][k] = v
		}
	}
	if val, ok := (*jobSpec)["volume"]; ok {
		for k, v := range val {
			(*oJobSpec)["volume"][k] = v
		}
	}
	if val, ok := (*jobSpec)["ingress"]; ok {
		for k, v := range val {
			(*oJobSpec)["ingress"][k] = v
		}
	}
	nJobBytes, err := json.Marshal(*oJobSpec)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	var nJobSpec v1beta1.JobSpec
	if err = json.Unmarshal(nJobBytes, &nJobSpec); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	return &nJobSpec
}

func (m *Manager) createJob(c *gin.Context) {
	mirrorID := c.Param("id")

	var e error
	ojb := new(v1beta1.Job)
	m.rwmu.Lock()
	defer m.rwmu.Unlock()
	job := v1beta1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: v1beta1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      mirrorID,
			Namespace: m.namespace,
		},
	}
	if err := m.client.Get(c.Request.Context(), client.ObjectKey{Namespace: m.namespace, Name: mirrorID}, ojb); err != nil || ojb == nil {
		var jobSpec v1beta1.JobSpec
		c.BindJSON(&jobSpec)
		job.Spec = jobSpec
	} else {
		oJobBytes, err := json.Marshal(ojb.Spec)
		if err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}
		var oJobSpec map[string]map[string]interface{}
		if err = json.Unmarshal(oJobBytes, &oJobSpec); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}
		jobSpec := make(map[string]map[string]interface{})
		c.BindJSON(&jobSpec)
		job.Spec = *handleMerge(c, &oJobSpec, &jobSpec)
	}
	e = m.client.Patch(c.Request.Context(), &job, client.Apply, []client.PatchOption{client.ForceOwnership, client.FieldOwner("mirror-controller")}...)

	if e != nil {
		err := fmt.Errorf("failed to patch job %s: %s",
			mirrorID, e.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{_infoKey: "patch " + mirrorID + " succeed"})
}

// listJob respond with all jobs of specified mirrors
func (m *Manager) listJob(c *gin.Context) {
	var ws []internal.MirrorStatus

	m.rwmu.RLock()
	defer m.rwmu.RUnlock()
	jobs := new(v1beta1.JobList)
	err := m.client.List(c.Request.Context(), jobs)

	for _, v := range jobs.Items {
		w := internal.MirrorStatus{ID: v.Name, Alias: v.Spec.Config.Alias, JobStatus: v.Status}
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

	m.rwmu.RLock()
	defer m.rwmu.RUnlock()

	job, err := m.GetJob(c, mirrorID)
	if err != nil {
		err := fmt.Errorf("failed to get job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, internal.MirrorStatus{ID: mirrorID, Alias: job.Spec.Config.Alias, JobStatus: job.Status})
}

func (m *Manager) getJobConfig(c *gin.Context) {
	mirrorID := c.Param("id")
	var config internal.MirrorConfig

	m.rwmu.RLock()
	defer m.rwmu.RUnlock()
	job, err := m.GetJob(c, mirrorID)
	config = internal.MirrorConfig{ID: mirrorID, JobSpec: job.Spec}

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
	runLog.Info(fmt.Sprintf("Geting log from <%s>", mirrorID))
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
	defer m.rwmu.Unlock()
	job, err := m.GetJob(c, mirrorID)

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
	defer m.rwmu.Unlock()
	job, err := m.GetJob(c, mirrorID)

	if err != nil {
		runLog.Error(err, fmt.Sprintf("Failed to get job %s: %s", mirrorID, err.Error()))
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	job.Status.LastOnline = time.Now().Unix()
	job.Status.LastRegister = time.Now().Unix()
	err = m.client.Status().Update(c.Request.Context(), job)
	if err != nil {
		err := fmt.Errorf("failed to register mirror %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	runLog.Info(fmt.Sprintf("Mirror <%s> registered", mirrorID))
	c.JSON(http.StatusOK, job.Status)
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
	defer m.rwmu.Unlock()
	curJob, err := m.GetJob(c, mirrorID)

	if err != nil {
		runLog.Error(err, fmt.Sprintf("failed to get job %s: %s", mirrorID, err.Error()))
		c.JSON(http.StatusOK, empty{})
	}

	if curJob.Status.Scheduled == schedule.NextSchedule {
		// no changes, skip update
		c.JSON(http.StatusOK, empty{})
	}

	curJob.Status.Scheduled = schedule.NextSchedule
	curJob.Status.LastOnline = time.Now().Unix()
	err = m.client.Status().Update(c.Request.Context(), curJob)
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
	defer m.rwmu.Unlock()
	curJob, err := m.GetJob(c, mirrorID)

	curTime := time.Now().Unix()

	status.LastOnline = curTime
	status.LastRegister = curJob.Status.LastRegister

	if status.Status == v1beta1.PreSyncing && curJob.Status.Status != v1beta1.PreSyncing {
		status.LastStarted = curTime
	} else {
		status.LastStarted = curJob.Status.LastStarted
	}
	// Only successful syncing needs last_update
	if status.Status == v1beta1.Success {
		status.LastUpdate = curTime
	} else {
		status.LastUpdate = curJob.Status.LastUpdate
	}
	if status.Status == v1beta1.Success || status.Status == v1beta1.Failed {
		status.LastEnded = curTime
	} else {
		status.LastEnded = curJob.Status.LastEnded
	}

	// Only message with meaningful size updates the mirror size
	if len(curJob.Status.Size) > 0 && curJob.Status.Size != "unknown" {
		if len(status.Size) == 0 || status.Size == "unknown" {
			status.Size = curJob.Status.Size
		}
	}

	// for logging
	switch status.Status {
	case v1beta1.Syncing:
		runLog.Info(fmt.Sprintf("Job [%s] starts syncing", status.ID))
	default:
		runLog.Info(fmt.Sprintf("Job [%s] %s", status.ID, status.Status))
	}

	curJob.Status = status.JobStatus
	err = m.client.Status().Update(c.Request.Context(), curJob)
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
	defer m.rwmu.Unlock()
	job, err := m.GetJob(c, mirrorID)

	if err != nil {
		runLog.Error(err, fmt.Sprintf("Failed to get status of job %s: %s", mirrorID, err.Error()))
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	// Only message with meaningful size updates the mirror size
	if len(msg.Size) > 0 || msg.Size != "unknown" {
		job.Status.Size = msg.Size
	}

	runLog.Info(fmt.Sprintf("Mirror size of [%s]: %s", mirrorID, job.Status.Size))

	job.Status.LastOnline = time.Now().Unix()
	err = m.client.Status().Update(c.Request.Context(), job)
	if err != nil {
		err := fmt.Errorf("failed to update job %s: %s",
			mirrorID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, job)
}

func (m *Manager) enableJob(c *gin.Context) {
	mirrorID := c.Param("id")

	m.rwmu.Lock()
	defer m.rwmu.Unlock()
	curJob, err := m.GetJob(c, mirrorID)

	if err != nil {
		runLog.Error(err, fmt.Sprintf("failed to get job %s: %s", mirrorID, err.Error()))
		return
	}

	curJob.Status.Status = v1beta1.None
	curJob.Status.LastOnline = time.Now().Unix()
	err = m.client.Status().Update(c.Request.Context(), curJob)

	if err != nil {
		err := fmt.Errorf("failed to enable mirror: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	runLog.Info(fmt.Sprintf("Mirror <%s> enabled", mirrorID))
	c.JSON(http.StatusOK, gin.H{_infoKey: "enabled"})
}

func (m *Manager) disableJob(c *gin.Context) {
	mirrorID := c.Param("id")

	m.rwmu.Lock()
	defer m.rwmu.Unlock()
	curJob, err := m.GetJob(c, mirrorID)

	if err != nil {
		runLog.Error(err, fmt.Sprintf("failed to get job %s: %s", mirrorID, err.Error()))
		return
	}

	curJob.Status.Status = v1beta1.Disabled
	curJob.Status.LastOnline = time.Now().Unix()
	err = m.client.Status().Update(c.Request.Context(), curJob)
	if err != nil {
		err := fmt.Errorf("failed to disable mirror: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	runLog.Info(fmt.Sprintf("Mirror <%s> disabled", mirrorID))
	c.JSON(http.StatusOK, gin.H{_infoKey: "disabled"})
}

// PostJSON posts json object to url
func (m *Manager) PostJSON(mirrorID string, obj interface{}) (*http.Response, error) {
	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(obj); err != nil {
		return nil, err
	}
	return m.httpClient.Post(fmt.Sprintf("http://%s:6000", mirrorID), "application/json; charset=utf-8", b)
}

func (m *Manager) handleClientCmd(c *gin.Context) {
	mirrorID := c.Param("id")
	var clientCmd internal.ClientCmd
	c.BindJSON(&clientCmd)

	switch clientCmd.Cmd {
	case internal.CmdStop:
		m.rwmu.Lock()
		defer m.rwmu.Unlock()
		curJob, err := m.GetJob(c, mirrorID)
		if err != nil {
			runLog.Error(err, fmt.Sprintf("failed to get job %s: %s", mirrorID, err.Error()))
			return
		}

		curJob.Status.Status = v1beta1.Paused
		curJob.Status.LastOnline = time.Now().Unix()
		err = m.client.Status().Update(c.Request.Context(), curJob)
		if err != nil {
			runLog.Error(err, fmt.Sprintf("failed to update job %s: %s", mirrorID, err.Error()))
			return
		}
	}

	runLog.Info(fmt.Sprintf("Posting command '%s' to <%s>", clientCmd.Cmd, mirrorID))
	// post command to mirror
	r, err := m.PostJSON(mirrorID, clientCmd)
	if err != nil {
		err := fmt.Errorf("post command to mirror %s fail: %s", mirrorID, err.Error())
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	if r.StatusCode == 200 {
		c.JSON(http.StatusOK, gin.H{_infoKey: "successfully send command to mirror " + mirrorID})
	} else {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			c.Error(err)
			m.returnErrJSON(c, http.StatusInternalServerError, err)
			return
		}
		c.String(r.StatusCode, string(body))
	}
}
