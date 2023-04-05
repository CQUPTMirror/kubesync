package worker

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/ztelliot/kubesync/api/v1beta1"
	. "github.com/ztelliot/kubesync/internal"
)

// A Worker is a instance of tunasync worker
type Worker struct {
	L   sync.Mutex
	cfg *Config
	job *mirrorJob

	managerChan chan jobMessage
	semaphore   chan empty
	exit        chan empty

	schedule   *scheduleQueue
	httpEngine *gin.Engine
	httpClient *http.Client
}

// NewTUNASyncWorker creates a worker
func NewTUNASyncWorker(cfg *Config) *Worker {

	if cfg.Global.Retry == 0 {
		cfg.Global.Retry = defaultMaxRetry
	}

	w := &Worker{
		cfg: cfg,

		managerChan: make(chan jobMessage, 32),
		semaphore:   make(chan empty, cfg.Global.Concurrent),
		exit:        make(chan empty),

		schedule: newScheduleQueue(),
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
	for _, mirror := range w.cfg.Mirrors {
		// Create Provider
		provider := newMirrorProvider(mirror, w.cfg)
		w.job = newMirrorJob(provider)
	}
}

// Ctrl server receives commands from the manager
func (w *Worker) makeHTTPServer() {
	s := gin.New()
	s.Use(gin.Recovery())

	s.POST("/", func(c *gin.Context) {
		w.L.Lock()
		defer w.L.Unlock()

		var cmd ClientCmd

		if err := c.BindJSON(&cmd); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"msg": "Invalid request"})
			return
		}

		logger.Noticef("Received command: %v", cmd)

		// No matter what command, the existing job
		// schedule should be flushed
		w.schedule.Remove(w.job.Name())

		// if job disabled, start them first
		switch cmd.Cmd {
		case CmdStart, CmdRestart:
			if w.job.State() == stateDisabled {
				go w.job.Run(w.managerChan, w.semaphore)
			}
		}
		switch cmd.Cmd {
		case CmdStart:
			if cmd.Force {
				w.job.ctrlChan <- jobForceStart
			} else {
				w.job.ctrlChan <- jobStart
			}
		case CmdRestart:
			w.job.ctrlChan <- jobRestart
		case CmdStop:
			// if job is disabled, no goroutine would be there
			// receiving this signal
			if w.job.State() != stateDisabled {
				w.job.ctrlChan <- jobStop
			}
		case CmdPing:
			// empty
		default:
			c.JSON(http.StatusNotAcceptable, gin.H{"msg": "Invalid Command"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"msg": "OK"})
	})
	w.httpEngine = s
}

func (w *Worker) runHTTPServer() {
	addr := fmt.Sprintf("%s:%d", w.cfg.Server.Addr, w.cfg.Server.Port)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      w.httpEngine,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if w.cfg.Server.SSLCert == "" && w.cfg.Server.SSLKey == "" {
		if err := httpServer.ListenAndServe(); err != nil {
			panic(err)
		}
	} else {
		if err := httpServer.ListenAndServeTLS(w.cfg.Server.SSLCert, w.cfg.Server.SSLKey); err != nil {
			panic(err)
		}
	}
}

func (w *Worker) runSchedule() {
	w.L.Lock()

	mirrorList := w.fetchJobStatus()

	// Fetch mirror list stored in the manager
	// put it on the scheduled time
	// if it's disabled, ignore it
	for _, m := range mirrorList {
		switch m.Status {
		case Disabled:
			w.job.SetState(stateDisabled)
			continue
		case Paused:
			w.job.SetState(statePaused)
			go w.job.Run(w.managerChan, w.semaphore)
			continue
		default:
			w.job.SetState(stateNone)
			go w.job.Run(w.managerChan, w.semaphore)
			stime := m.LastUpdate + int64(w.job.provider.Interval().Seconds())
			// logger.Debugf("Scheduling job %s @%s", w.job.Name(), stime.Format("2006-01-02 15:04:05"))
			w.schedule.AddJob(stime, w.job)
		}
	}
	// some new jobs may be added
	// which does not exist in the
	// manager's mirror list
	w.job.SetState(stateNone)
	go w.job.Run(w.managerChan, w.semaphore)
	w.schedule.AddJob(time.Now().Unix(), w.job)

	w.L.Unlock()

	schedInfo := w.schedule.GetJobs()
	w.updateSchedInfo(schedInfo)

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

			schedInfo = w.schedule.GetJobs()
			w.updateSchedInfo(schedInfo)

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
					if jobMsg.status == Failed || jobMsg.status == Success {
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
	return w.cfg.Global.Name
}

// URL returns the url to http server of the worker
func (w *Worker) URL() string {
	proto := "https"
	if w.cfg.Server.SSLCert == "" && w.cfg.Server.SSLKey == "" {
		proto = "http"
	}

	return fmt.Sprintf("%s://%s:%d/", proto, w.cfg.Server.Hostname, w.cfg.Server.Port)
}

func (w *Worker) registerWorker() {
	msg := MirrorStatus{MirrorBase: MirrorBase{ID: w.Name()}}

	for _, root := range w.cfg.Manager.APIBaseList() {
		url := fmt.Sprintf("%s/jobs", root)
		logger.Debugf("register on manager url: %s", url)
		for retry := 10; retry > 0; {
			if _, err := PostJSON(url, msg, w.httpClient); err != nil {
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
}

func (w *Worker) updateStatus(job *mirrorJob, jobMsg jobMessage) {
	p := job.provider
	smsg := MirrorStatus{
		MirrorBase: MirrorBase{ID: w.cfg.Global.Name},
		JobStatus:  JobStatus{Status: jobMsg.status, Upstream: p.Upstream(), Size: "unknown", ErrorMsg: jobMsg.msg},
	}

	// Certain Providers (rsync for example) may know the size of mirror,
	// so we report it to Manager here
	if len(job.size) != 0 {
		smsg.Size = job.size
	}

	for _, root := range w.cfg.Manager.APIBaseList() {
		url := fmt.Sprintf(
			"%s/jobs/%s", root, w.Name(),
		)
		logger.Debugf("reporting on manager url: %s", url)
		if _, err := PostJSON(url, smsg, w.httpClient); err != nil {
			logger.Errorf("Failed to update mirror(%s) status: %s", w.Name(), err.Error())
		}
	}
}

func (w *Worker) updateSchedInfo(schedInfo []jobScheduleInfo) {
	var s []MirrorSchedule
	for _, sched := range schedInfo {
		s = append(s, MirrorSchedule{
			MirrorBase:   MirrorBase{ID: sched.jobName},
			NextSchedule: sched.nextScheduled.Unix(),
		})
	}
	msg := MirrorSchedules{Schedules: s}

	for _, root := range w.cfg.Manager.APIBaseList() {
		url := fmt.Sprintf(
			"%s/jobs/%s/schedules", root, w.Name(),
		)
		logger.Debugf("reporting on manager url: %s", url)
		if _, err := PostJSON(url, msg, w.httpClient); err != nil {
			logger.Errorf("Failed to upload schedules: %s", err.Error())
		}
	}
}

func (w *Worker) fetchJobStatus() []MirrorStatus {
	var mirrorList []MirrorStatus
	apiBase := w.cfg.Manager.APIBaseList()[0]

	url := fmt.Sprintf("%s/jobs/%s", apiBase, w.Name())

	if _, err := GetJSON(url, &mirrorList, w.httpClient); err != nil {
		logger.Errorf("Failed to fetch job status: %s", err.Error())
	}

	return mirrorList
}
