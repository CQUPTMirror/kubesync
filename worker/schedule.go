package worker

// schedule queue for jobs

import (
	"sync"
	"time"
)

type schedule struct {
	sync.Mutex
	job           *mirrorJob
	nextScheduled int64
	sched         bool
}

func newSchedule() *schedule {
	queue := new(schedule)
	return queue
}

func (q *schedule) GetJob() (nextScheduled int64) {
	if q.sched {
		nextScheduled = q.nextScheduled
	}
	return
}

func (q *schedule) AddJob(schedTime int64, job *mirrorJob) {
	q.Lock()
	defer q.Unlock()
	if q.sched {
		logger.Warningf("Job %s already scheduled, removing the existing one", job.Name())
		q.Remove()
	}
	q.job = job
	q.sched = true
	q.nextScheduled = schedTime
	logger.Debugf("Added job %s @ %v", job.Name(), time.Unix(q.nextScheduled, 0))
}

// Pop out the first job if it's time to run it
func (q *schedule) Pop() *mirrorJob {
	q.Lock()
	defer q.Unlock()

	if !q.sched {
		return nil
	}

	t := time.Unix(q.nextScheduled, 0)
	if t.Before(time.Now()) {
		job := q.job
		q.sched = false
		logger.Debug("Popped out job %s @%v", job.Name(), t)
		return job
	}
	return nil
}

// remove job
func (q *schedule) Remove() {
	q.Lock()
	defer q.Unlock()
	q.sched = false
	return
}
