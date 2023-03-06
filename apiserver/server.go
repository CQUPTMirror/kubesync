package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ztelliot/kubesync/api/v1beta1"
)

const (
	_errorKey = "error"
	_infoKey  = "message"
)

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

// Run runs the manager server forever
func (s *Manager) Run(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
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

// listAllJobs respond with all jobs of specified mirrors
func (s *Manager) listAllJobs(c *gin.Context) {
	mirrorStatusList, err := s.adapter.ListJobs(c.Request.Context())
	if err != nil {
		err := fmt.Errorf("failed to list mirrors: %s",
			err.Error(),
		)
		c.Error(err)
		s.returnErrJSON(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, mirrorStatusList)
}

// deleteJob deletes one job by id
func (s *Manager) deleteJob(c *gin.Context) {
	mirrorID := c.Param("id")
	err := s.adapter.DeleteJob(c.Request.Context(), mirrorID)
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

func (s *Manager) getJob(c *gin.Context) {
	mirrorID := c.Param("id")
	var status MirrorStatus
	c.BindJSON(&status)

	status, err := s.adapter.GetJob(c.Request.Context(), mirrorID)

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

// registerMirror register an newly-online mirror
func (s *Manager) registerMirror(c *gin.Context) {
	var _mirror MirrorStatus
	c.BindJSON(&_mirror)
	_mirror.LastOnline = time.Now().Unix()
	_mirror.LastRegister = time.Now().Unix()
	err := s.adapter.UpdateJobStatus(c.Request.Context(), _mirror)
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

func (s *Manager) updateSchedules(c *gin.Context) {
	var schedules MirrorSchedules
	c.BindJSON(&schedules)

	for _, schedule := range schedules.Schedules {
		mirrorID := schedule.MirrorID
		if len(mirrorID) == 0 {
			s.returnErrJSON(
				c, http.StatusBadRequest,
				errors.New("Mirror Name should not be empty"),
			)
		}

		curStatus, err := s.adapter.GetJob(c.Request.Context(), mirrorID)
		if err != nil {
			runLog.Error(err, "failed to get job %s: %s",
				mirrorID, err.Error(),
			)
			continue
		}

		if curStatus.Scheduled == schedule.NextSchedule {
			// no changes, skip update
			continue
		}

		curStatus.Scheduled = schedule.NextSchedule
		err = s.adapter.UpdateJobStatus(c.Request.Context(), curStatus)
		if err != nil {
			err := fmt.Errorf("failed to update job %s: %s",
				mirrorID, err.Error(),
			)
			c.Error(err)
			s.returnErrJSON(c, http.StatusInternalServerError, err)
			return
		}
	}
	type empty struct{}
	c.JSON(http.StatusOK, empty{})
}

func (s *Manager) updateJob(c *gin.Context) {
	mirrorID := c.Param("id")
	var status MirrorStatus
	c.BindJSON(&status)

	curStatus, err := s.adapter.GetJob(c.Request.Context(), mirrorID)

	curTime := time.Now().Unix()

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

	err = s.adapter.UpdateJobStatus(c.Request.Context(), status)
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
	mirrorID := c.Param("id")
	type SizeMsg struct {
		ID   string `json:"id"`
		Size string `json:"size"`
	}
	var msg SizeMsg
	c.BindJSON(&msg)

	mirrorName := msg.ID
	status, err := s.adapter.GetJob(c.Request.Context(), mirrorID)
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

	err = s.adapter.UpdateJobStatus(c.Request.Context(), status)
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
	mirrorID := c.Param("id")
	var clientCmd ClientCmd
	c.BindJSON(&clientCmd)
	if mirrorID == "" {
		// TODO: decide which mirror should do this mirror when MirrorID is null string
		runLog.Info("handleClientCmd case mirrorID == \" \" not implemented yet")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	curStat, err := s.adapter.GetJob(c.Request.Context(), mirrorID)
	changed := false
	switch clientCmd.Cmd {
	case CmdDisable:
		curStat.Status = v1beta1.Disabled
		changed = true
	case CmdStop:
		curStat.Status = v1beta1.Paused
		changed = true
	}
	if changed {
		s.adapter.UpdateJobStatus(c.Request.Context(), curStat)
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
