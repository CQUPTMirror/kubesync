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
	"github.com/CQUPTMirror/kubesync/manager/mirrorz"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/internal"
	"github.com/CQUPTMirror/kubesync/manager/external"
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
	MirrorZ *mirrorz.MirrorZ
	Total   string
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
	option     *Options
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
		option:     &options,
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
	s.engine.GET("/api/mirrors", s.listJob)

	if options.MirrorZ != nil {
		s.engine.GET("/api/mirrorz.json", s.mirrorZ)
	}

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
		mirrorValidateGroup.POST("", s.createJob)
		// mirror online
		mirrorValidateGroup.HEAD("", s.registerMirror)
		// post job status
		mirrorValidateGroup.PATCH("", s.updateJob)
		mirrorValidateGroup.POST("size", s.updateMirrorSize)
		mirrorValidateGroup.POST("schedule", s.updateSchedule)
		mirrorValidateGroup.POST("enable", s.enableJob)
		mirrorValidateGroup.POST("disable", s.disableJob)
		// for tunasynctl to post commands
		mirrorValidateGroup.POST("cmd", s.handleClientCmd)
	}

	// list announcements
	s.engine.GET("/announcements", s.listAnnouncement)
	s.engine.GET("/api/news", s.listAnnouncement)

	// announcementID should be valid in this route group
	announcementValidateGroup := s.engine.Group("/announcement/:id")
	{
		// create or patch announcement
		announcementValidateGroup.POST("", s.createAnnouncement)
		// delete specified announcement
		announcementValidateGroup.DELETE("", s.deleteAnnouncement)
		// get announcement detail
		announcementValidateGroup.GET("", s.getAnnouncement)
	}

	// list files
	s.engine.GET("/files", s.listFile)
	s.engine.GET("/api/files", s.listFile)

	// fileID should be valid in this route group
	fileValidateGroup := s.engine.Group("/file/:id")
	{
		// create or patch file
		fileValidateGroup.POST("", s.updateFile)
		// delete specified file
		fileValidateGroup.DELETE("", s.deleteFile)
		// get file detail
		fileValidateGroup.GET("", s.getFile)
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
		if v.Spec.Config.Type == v1beta1.External {
			wss, _ := external.Provider(&v.Spec.Config, m.httpClient).List()
			ws = append(ws, wss...)
		} else {
			w := internal.MirrorStatus{
				ID:        v.Name,
				Alias:     v.Spec.Config.Alias,
				Desc:      v.Spec.Config.Desc,
				Url:       v.Spec.Config.Url,
				HelpUrl:   v.Spec.Config.HelpUrl,
				Type:      v.Spec.Config.Type,
				SizeStr:   internal.ParseSize(v.Status.Size),
				JobStatus: v.Status,
			}
			switch v.Spec.Config.Type {
			case v1beta1.Proxy:
				w.Upstream = v.Spec.Config.Upstream
				w.Status = v1beta1.Cached
			case v1beta1.Git:
				w.Upstream = v.Spec.Config.Upstream
				w.Status = v1beta1.Created
			case "":
				w.Type = v1beta1.Mirror
			}
			ws = append(ws, w)
		}
	}

	sort.Slice(ws, func(i, j int) bool {
		return strings.ToLower(ws[i].ID) < strings.ToLower(ws[j].ID)
	})

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
	c.JSON(http.StatusOK, job.Status)
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
	var status v1beta1.JobStatus
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
	if curJob.Status.Size > 0 {
		if status.Size == 0 {
			status.Size = curJob.Status.Size
		}
	}

	// for logging
	switch status.Status {
	case v1beta1.Syncing:
		runLog.Info(fmt.Sprintf("Job [%s] starts syncing", mirrorID))
	default:
		runLog.Info(fmt.Sprintf("Job [%s] %s", mirrorID, status.Status))
	}

	curJob.Status = status
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
		Size uint64 `json:"size"`
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

	job.Status.Size = msg.Size
	runLog.Info(fmt.Sprintf("Mirror size of [%s]: %d", mirrorID, job.Status.Size))

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

	curJob.Status.Status = v1beta1.Created
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

func (m *Manager) GetAnnouncement(c *gin.Context, announcementID string) (*v1beta1.Announcement, error) {
	news := new(v1beta1.Announcement)
	err := m.client.Get(c.Request.Context(), client.ObjectKey{Namespace: m.namespace, Name: announcementID}, news)
	if err != nil {
		err := fmt.Errorf("failed to get announcement: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return nil, err
	}
	return news, err
}

func (m *Manager) createAnnouncement(c *gin.Context) {
	announcementID := c.Param("id")

	var e error
	oNews := new(v1beta1.Announcement)
	m.rwmu.Lock()
	defer m.rwmu.Unlock()
	news := v1beta1.Announcement{
		TypeMeta:   metav1.TypeMeta{Kind: "Announcement", APIVersion: v1beta1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: announcementID, Namespace: m.namespace},
	}
	if err := m.client.Get(c.Request.Context(), client.ObjectKey{Namespace: m.namespace, Name: announcementID}, oNews); err != nil || oNews == nil {
		var newsSpec v1beta1.AnnouncementSpec
		c.BindJSON(&newsSpec)
		news.Spec = newsSpec
	} else {
		newsSpec := make(map[string]string)
		c.BindJSON(&newsSpec)
		if v, ok := newsSpec["title"]; ok {
			oNews.Spec.Title = v
		}
		if v, ok := newsSpec["content"]; ok {
			oNews.Spec.Content = v
		}
		if v, ok := newsSpec["author"]; ok {
			oNews.Spec.Author = v
		}
		news.Spec = oNews.Spec
	}

	e = m.client.Patch(c.Request.Context(), &news, client.Apply, []client.PatchOption{client.ForceOwnership, client.FieldOwner("mirror-controller")}...)
	if e != nil {
		err := fmt.Errorf("failed to patch announcement %s: %s",
			announcementID, e.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{_infoKey: "patch " + announcementID + " succeed"})
}

// listAnnouncement respond with all announcements
func (m *Manager) listAnnouncement(c *gin.Context) {
	var ws []internal.AnnouncementInfo

	m.rwmu.RLock()
	defer m.rwmu.RUnlock()
	announcements := new(v1beta1.AnnouncementList)
	err := m.client.List(c.Request.Context(), announcements)

	for _, v := range announcements.Items {
		ws = append(ws, internal.AnnouncementInfo{
			ID:                 v.Name,
			Title:              v.Spec.Title,
			Author:             v.Spec.Author,
			Content:            v.Spec.Content,
			AnnouncementStatus: v.Status,
		})
	}

	sort.Slice(ws, func(i, j int) bool {
		return ws[i].ID < ws[j].ID
	})

	if err != nil {
		err := fmt.Errorf("failed to list announcements: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (m *Manager) getAnnouncement(c *gin.Context) {
	announcementID := c.Param("id")

	m.rwmu.RLock()
	defer m.rwmu.RUnlock()

	announcement, err := m.GetAnnouncement(c, announcementID)
	if err != nil {
		err := fmt.Errorf("failed to get announcement %s: %s",
			announcementID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, internal.AnnouncementInfo{
		ID:                 announcementID,
		Title:              announcement.Spec.Title,
		Author:             announcement.Spec.Author,
		Content:            announcement.Spec.Content,
		AnnouncementStatus: announcement.Status,
	})
}

// deleteAnnouncement deletes one announcement by id
func (m *Manager) deleteAnnouncement(c *gin.Context) {
	announcementID := c.Param("id")

	m.rwmu.Lock()
	defer m.rwmu.Unlock()
	news, err := m.GetAnnouncement(c, announcementID)

	if err != nil {
		return
	}
	err = m.client.Delete(c.Request.Context(), news)
	if err != nil {
		err := fmt.Errorf("failed to delete announcement: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	runLog.Info(fmt.Sprintf("Announcement <%s> deleted", announcementID))
	c.JSON(http.StatusOK, gin.H{_infoKey: "deleted"})
}

func (m *Manager) GetFile(c *gin.Context, fileID string) (*v1beta1.File, error) {
	file := new(v1beta1.File)
	err := m.client.Get(c.Request.Context(), client.ObjectKey{Namespace: m.namespace, Name: fileID}, file)
	if err != nil {
		err := fmt.Errorf("failed to get file: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return nil, err
	}
	return file, err
}

func (m *Manager) updateFile(c *gin.Context) {
	fileID := c.Param("id")

	oFile := new(v1beta1.File)
	var nFile internal.FileBase
	c.BindJSON(&nFile)

	var fileInfo []v1beta1.FileInfo
	if len(nFile.Files) > 0 {
		for _, v := range nFile.Files {
			info := internal.Recognizer(v)
			if info.Name != "" {
				fileInfo = append(fileInfo, info)
			}
		}
	}

	m.rwmu.Lock()
	defer m.rwmu.Unlock()
	file := v1beta1.File{
		TypeMeta:   metav1.TypeMeta{Kind: "File", APIVersion: v1beta1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: fileID, Namespace: m.namespace},
		Spec:       v1beta1.FileSpec{Type: nFile.Type, Alias: nFile.Alias},
		Status:     v1beta1.FileStatus{},
	}

	if err := m.client.Get(c.Request.Context(), client.ObjectKey{Namespace: m.namespace, Name: fileID}, oFile); err != nil || oFile == nil {
		if file.Spec.Type == "" {
			file.Spec.Type = v1beta1.OS
		}
	} else {
		if file.Spec.Type == "" {
			file.Spec.Type = oFile.Spec.Type
		}

		if file.Spec.Alias == "" {
			file.Spec.Alias = oFile.Spec.Alias
		}
	}

	if file.Spec.Type != oFile.Spec.Type || file.Spec.Alias != oFile.Spec.Alias {
		e := m.client.Patch(c.Request.Context(), &file, client.Apply, []client.PatchOption{client.ForceOwnership, client.FieldOwner("mirror-controller")}...)
		if e != nil {
			err := fmt.Errorf("failed to patch file %s info: %s",
				fileID, e.Error(),
			)
			c.Error(err)
			m.returnErrJSON(c, http.StatusInternalServerError, err)
			return
		}
		if len(fileInfo) > 0 {
			if e := m.client.Get(c.Request.Context(), client.ObjectKey{Namespace: m.namespace, Name: fileID}, oFile); e != nil {
				err := fmt.Errorf("failed to get file: %s",
					e.Error(),
				)
				c.Error(err)
				m.returnErrJSON(c, http.StatusInternalServerError, err)
				return
			}
		}
	}

	if len(fileInfo) > 0 {
		oFile.Status.Files = fileInfo
		oFile.Status.UpdateTime = time.Now().Unix()

		e := m.client.Status().Update(c.Request.Context(), oFile)
		if e != nil {
			err := fmt.Errorf("failed to update file %s list: %s",
				fileID, e.Error(),
			)
			c.Error(err)
			m.returnErrJSON(c, http.StatusInternalServerError, err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{_infoKey: "update " + fileID + " succeed"})
}

// listFile respond with all files
func (m *Manager) listFile(c *gin.Context) {
	var ws []internal.FileInfo

	m.rwmu.RLock()
	defer m.rwmu.RUnlock()
	files := new(v1beta1.FileList)
	err := m.client.List(c.Request.Context(), files)

	for _, v := range files.Items {
		if len(v.Status.Files) > 0 {
			ws = append(ws, internal.FileInfo{ID: v.Name, Type: v.Spec.Type, Alias: v.Spec.Alias, FileStatus: v.Status})
		}
	}

	sort.Slice(ws, func(i, j int) bool {
		return ws[i].ID < ws[j].ID
	})

	if err != nil {
		err := fmt.Errorf("failed to list files: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (m *Manager) getFile(c *gin.Context) {
	fileID := c.Param("id")

	m.rwmu.RLock()
	defer m.rwmu.RUnlock()

	file, err := m.GetFile(c, fileID)
	if err != nil {
		err := fmt.Errorf("failed to get file %s: %s",
			fileID, err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, internal.FileInfo{ID: fileID, Type: file.Spec.Type, Alias: file.Spec.Alias, FileStatus: file.Status})
}

// deleteFile deletes one file by id
func (m *Manager) deleteFile(c *gin.Context) {
	fileID := c.Param("id")

	m.rwmu.Lock()
	defer m.rwmu.Unlock()
	file, err := m.GetFile(c, fileID)

	if err != nil {
		return
	}
	err = m.client.Delete(c.Request.Context(), file)
	if err != nil {
		err := fmt.Errorf("failed to delete file: %s",
			err.Error(),
		)
		c.Error(err)
		m.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	runLog.Info(fmt.Sprintf("File <%s> deleted", fileID))
	c.JSON(http.StatusOK, gin.H{_infoKey: "deleted"})
}

func (m *Manager) mirrorZ(c *gin.Context) {
	mirrorZ := m.option.MirrorZ

	files := new(v1beta1.FileList)
	if err := m.client.List(c.Request.Context(), files); err != nil {
		for _, v := range files.Items {
			if len(v.Status.Files) > 0 {
				distro := v.Spec.Alias
				if distro == "" {
					distro = v.Name
				}
				var urls []mirrorz.InfoUrl
				for _, u := range v.Status.Files {
					urls = append(urls, mirrorz.InfoUrl{Name: u.Name, Url: u.Path})
				}
				mirrorZ.Info = append(mirrorZ.Info, mirrorz.Info{Distro: distro, Category: string(v.Spec.Type), Urls: urls})
			}
		}
	}

	var fullSize uint64 = 0
	jobs := new(v1beta1.JobList)
	if err := m.client.List(c.Request.Context(), jobs); err != nil {
		for _, v := range jobs.Items {
			if v.Spec.Config.Type == v1beta1.External {
				ws, _ := external.Provider(&v.Spec.Config, m.httpClient).ListZ()
				mirrorZ.Mirrors = append(mirrorZ.Mirrors, ws...)
			} else {
				fullSize += v.Status.Size
				disabled := false
				cname := v.Spec.Config.Alias
				if cname == "" {
					cname = v.Name
				}
				url := v.Spec.Config.Url
				if url == "" {
					url = fmt.Sprintf("/%s", v.Name)
				}
				status := "U"
				switch v.Spec.Config.Type {
				case v1beta1.Proxy:
					status = "C"
				default:
					switch v.Status.Status {
					case v1beta1.Success:
						if v.Status.LastUpdate != 0 {
							status = fmt.Sprintf("S%d", v.Status.LastUpdate)
						}
					case v1beta1.PreSyncing:
						if v.Status.Scheduled != 0 {
							status = fmt.Sprintf("D%d", v.Status.Scheduled)
						}
					case v1beta1.Syncing:
						if v.Status.LastStarted != 0 {
							status = fmt.Sprintf("Y%d", v.Status.LastStarted)
						}
					case v1beta1.Failed:
						if v.Status.LastEnded != 0 {
							status = fmt.Sprintf("F%d", v.Status.LastEnded)
						}
					case v1beta1.Paused:
						if v.Status.LastEnded != 0 {
							status = fmt.Sprintf("P%d", v.Status.LastEnded)
						}
					case v1beta1.Created:
						if v.Status.LastRegister != 0 {
							status = fmt.Sprintf("N%d", v.Status.LastRegister)
						}
					case v1beta1.Disabled:
						disabled = true
					}
					if status != "U" {
						if v.Status.Scheduled != 0 {
							status += fmt.Sprintf("X%d", v.Status.Scheduled)
						}
						if v.Status.LastUpdate == 0 && v.Status.LastRegister != 0 {
							status += fmt.Sprintf("N%d", v.Status.LastRegister)
						}
						if v.Status.Status == v1beta1.Syncing || v.Status.Status == v1beta1.Failed {
							status += fmt.Sprintf("O%d", v.Status.LastUpdate)
						}
					}
				}
				w := mirrorz.Mirror{
					Cname:    cname,
					Desc:     v.Spec.Config.Desc,
					Url:      url,
					Status:   status,
					Help:     v.Spec.Config.HelpUrl,
					Upstream: v.Spec.Config.Upstream,
					Size:     internal.ParseSize(v.Status.Size),
					Disable:  disabled,
				}
				mirrorZ.Mirrors = append(mirrorZ.Mirrors, w)
			}
		}
	}

	mirrorZ.Site.Disk = internal.ParseSize(fullSize)
	if m.option.Total != "" {
		mirrorZ.Site.Disk += "/" + m.option.Total
	}

	c.JSON(http.StatusOK, mirrorZ)
}
