package apiserver

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ztelliot/kubesync/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type operatorAdapter interface {
	ListJobs(ctx context.Context) ([]MirrorStatus, error)
	GetJob(ctx context.Context, mirrorID string) (MirrorStatus, error)
	DeleteJob(ctx context.Context, mirrorID string) error
	CreateJob(ctx context.Context, c MirrorConfig) error
	UpdateJobStatus(ctx context.Context, status MirrorStatus) error
	RefreshJob(ctx context.Context, mirrorID string) error
}

func (m *Manager) ListMirrors(ctx context.Context) (ws []MirrorStatus, err error) {
	jobs := new(v1beta1.JobList)
	err = m.client.List(ctx, jobs, &client.ListOptions{Namespace: m.namespace})

	for _, v := range jobs.Items {
		w := MirrorStatus{ID: v.Name, JobStatus: v.Status}
		ws = append(ws, w)
	}
	return
}

func (m *Manager) GetJobRaw(ctx context.Context, mirrorID string) (*v1beta1.Job, error) {
	job := new(v1beta1.Job)
	err := m.client.Get(ctx, client.ObjectKey{Namespace: m.namespace, Name: mirrorID}, job)
	return job, err
}

func (m *Manager) GetJob(ctx context.Context, mirrorID string) (w MirrorStatus, err error) {
	job, err := m.GetJobRaw(ctx, mirrorID)
	w = MirrorStatus{ID: mirrorID, JobStatus: job.Status}
	return
}

func (m *Manager) DeleteJob(ctx context.Context, mirrorID string) error {
	job, err := m.GetJobRaw(ctx, mirrorID)
	if err != nil {
		return err
	}
	err = m.client.Delete(ctx, job)
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) CreateJob(ctx context.Context, c MirrorConfig) error {
	job := &v1beta1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: c.ID, Namespace: m.namespace},
		Spec:       c.JobSpec,
	}
	return m.client.Create(ctx, job)
}

func (m *Manager) UpdateJobStatus(ctx context.Context, w MirrorStatus) error {
	job, err := m.GetJobRaw(ctx, w.ID)
	if err != nil {
		return err
	}
	job.Status = w.JobStatus
	err = m.client.Update(ctx, job)
	return err
}

func (m *Manager) RefreshJob(ctx context.Context, mirrorID string) error {
	w, err := m.GetJob(ctx, mirrorID)
	if err == nil {
		w.LastOnline = time.Now().Unix()
		err = m.UpdateJobStatus(ctx, w)
	}
	return err
}
