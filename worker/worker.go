package worker

import (
	"fmt"
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/internal"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// A Worker is an instance of tunasync worker
type Worker struct {
	L   sync.Mutex
	cfg *Config
	job *mirrorJob

	managerChan chan jobMessage
	semaphore   chan empty
	exit        chan empty

	schedule   *schedule
	httpEngine *gin.Engine
	httpClient *http.Client
}

// NewTUNASyncWorker creates a worker
func NewTUNASyncWorker(cfg *Config) *Worker {

	if cfg.Retry == 0 {
		cfg.Retry = defaultMaxRetry
	}

	w := &Worker{
		cfg: cfg,

		managerChan: make(chan jobMessage, 32),
		semaphore:   make(chan empty, cfg.Concurrent),
		exit:        make(chan empty),

		schedule: newSchedule(),
	}

	w.initJobs()
	w.makeHTTPServer()
	return w
}

// Run runs worker forever
func (w *Worker) Run() {
	w.registerWorker()
	go w.runHTTPServer()
	w.runSchedule()
}

// Halt stops all jobs
func (w *Worker) Halt() {
	w.L.Lock()
	logger.Notice("Stopping all the jobs")
	if w.job.State() != stateDisabled {
		w.job.ctrlChan <- jobHalt
	}
	jobsDone.Wait()
	logger.Notice("All the jobs are stopped")
	w.L.Unlock()
	close(w.exit)
}

func (w *Worker) initJobs() {
	provider := newMirrorProvider(w.cfg)
	w.job = newMirrorJob(provider)
}

// Ctrl server receives commands from the manager
func (w *Worker) makeHTTPServer() {
	s := gin.New()
	s.Use(gin.Recovery())

	s.POST("/", func(c *gin.Context) {
		w.L.Lock()
		defer w.L.Unlock()

		var cmd internal.ClientCmd

		if err := c.BindJSON(&cmd); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"msg": "Invalid request"})
			return
		}

		logger.Noticef("Received command: %v", cmd)

		// No matter what command, the existing job
		// schedule should be flushed
		w.schedule.Remove()

		// if job disabled, start them first
		switch cmd.Cmd {
		case internal.CmdStart, internal.CmdRestart:
			if w.job.State() == stateDisabled {
				go w.job.Run(w.managerChan, w.semaphore)
			}
		}
		switch cmd.Cmd {
		case internal.CmdStart:
			if cmd.Force {
				w.job.ctrlChan <- jobForceStart
			} else {
				w.job.ctrlChan <- jobStart
			}
		case internal.CmdRestart:
			w.job.ctrlChan <- jobRestart
		case internal.CmdStop:
			// if job is disabled, no goroutine would be there
			// receiving this signal
			if w.job.State() != stateDisabled {
				w.job.ctrlChan <- jobStop
			}
		case internal.CmdPing:
			// empty
		default:
			c.JSON(http.StatusNotAcceptable, gin.H{"msg": "Invalid Command"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"msg": "OK"})
	})
	s.GET("/log", func(c *gin.Context) {
		logger.Noticef("Return latest log")
		filePath := filepath.Join(w.cfg.LogDir, "latest.log")
		_, err := os.Stat(filePath)
		if err != nil {
			c.String(http.StatusNotFound, "log not found")
			return
		}
		c.Header("Content-Type", "text/plain")
		c.File(filePath)
	})
	w.httpEngine = s
}

func (w *Worker) runHTTPServer() {
	httpServer := &http.Server{
		Addr:         w.cfg.Addr,
		Handler:      w.httpEngine,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if err := httpServer.ListenAndServe(); err != nil {
		panic(err)
	}
}

func (w *Worker) runSchedule() {
	w.L.Lock()

	mirror := w.fetchJobStatus()

	// Fetch mirror list stored in the manager
	// put it on the scheduled time
	// if it's disabled, ignore it
	switch mirror.Status {
	case v1beta1.Disabled:
		w.job.SetState(stateDisabled)
	case v1beta1.Paused:
		w.job.SetState(statePaused)
		go w.job.Run(w.managerChan, w.semaphore)
	default:
		w.job.SetState(stateNone)
		go w.job.Run(w.managerChan, w.semaphore)
		stime := mirror.LastUpdate + int64(w.job.provider.Interval().Seconds())
		// logger.Debugf("Scheduling job %s @%s", w.job.Name(), stime.Format("2006-01-02 15:04:05"))
		w.schedule.AddJob(stime, w.job)
	}

	w.L.Unlock()

	nextScheduled := w.schedule.GetJob()
	w.updateSchedInfo(nextScheduled)

	tick := time.Tick(5 * time.Second)
	for {
		select {
		case jobMsg := <-w.managerChan:
			// got status update from job
			if (w.job.State() != stateReady) && (w.job.State() != stateHalting) {
				logger.Infof("Job %s state is not ready, skip adding new schedule", w.Name())
				continue
			}

			// syncing status is only meaningful when job
			// is running. If it's paused or disabled
			// a sync failure signal would be emitted
			// which needs to be ignored
			w.updateStatus(w.job, jobMsg)

			// only successful or the final failure msg
			// can trigger scheduling
			if jobMsg.schedule {
				schedTime := time.Now().Add(w.job.provider.Interval())
				logger.Noticef(
					"Next scheduled time for %s: %s",
					w.job.Name(),
					schedTime.Format("2006-01-02 15:04:05"),
				)
				w.schedule.AddJob(schedTime.Unix(), w.job)
			}

			nextScheduled = w.schedule.GetJob()
			w.updateSchedInfo(nextScheduled)

		case <-tick:
			// check schedule every 5 seconds
			if job := w.schedule.Pop(); job != nil {
				job.ctrlChan <- jobStart
			}
		case <-w.exit:
			// flush status update messages
			w.L.Lock()
			defer w.L.Unlock()
			for {
				select {
				case jobMsg := <-w.managerChan:
					logger.Debugf("status update from %s", w.Name())
					if jobMsg.status == v1beta1.Failed || jobMsg.status == v1beta1.Success {
						w.updateStatus(w.job, jobMsg)
					}
				default:
					return
				}
			}
		}
	}
}

// Name returns worker name
func (w *Worker) Name() string {
	return w.cfg.Name
}

func (w *Worker) registerWorker() {
	url := fmt.Sprintf("%s/jobs/%s", w.cfg.APIBase, w.Name())
	logger.Debugf("register on manager url: %s", url)
	for retry := 10; retry > 0; {
		client := w.httpClient
		if client == nil {
			client, _ = CreateHTTPClient()
		}
		if _, err := client.Post(url, "application/json; charset=utf-8", nil); err != nil {
			logger.Errorf("Failed to register worker")
			retry--
			if retry > 0 {
				time.Sleep(1 * time.Second)
				logger.Noticef("Retrying... (%d)", retry)
			}
		} else {
			break
		}
	}
}

func (w *Worker) updateStatus(job *mirrorJob, jobMsg jobMessage) {
	p := job.provider
	smsg := internal.MirrorStatus{
		ID:        w.cfg.Name,
		JobStatus: v1beta1.JobStatus{Status: jobMsg.status, Upstream: p.Upstream(), Size: "unknown", ErrorMsg: jobMsg.msg},
	}

	// Certain Providers (rsync for example) may know the size of mirror,
	// so we report it to Manager here
	if len(job.size) != 0 {
		smsg.Size = job.size
	}

	url := fmt.Sprintf(
		"%s/jobs/%s", w.cfg.APIBase, w.Name(),
	)
	logger.Debugf("reporting on manager url: %s", url)
	if _, err := PatchJSON(url, smsg, w.httpClient); err != nil {
		logger.Errorf("Failed to update mirror(%s) status: %s", w.Name(), err.Error())
	}
}

func (w *Worker) updateSchedInfo(nextScheduled int64) {
	msg := internal.MirrorSchedule{NextSchedule: nextScheduled}

	url := fmt.Sprintf(
		"%s/jobs/%s/schedule", w.cfg.APIBase, w.Name(),
	)
	logger.Debugf("reporting on manager url: %s", url)
	if _, err := PostJSON(url, msg, w.httpClient); err != nil {
		logger.Errorf("Failed to upload schedule: %s", err.Error())
	}
}

func (w *Worker) fetchJobStatus() internal.MirrorStatus {
	var mirror internal.MirrorStatus

	url := fmt.Sprintf("%s/jobs/%s", w.cfg.APIBase, w.Name())

	if _, err := GetJSON(url, &mirror, w.httpClient); err != nil {
		logger.Errorf("Failed to fetch job status: %s", err.Error())
	}

	return mirror
}
