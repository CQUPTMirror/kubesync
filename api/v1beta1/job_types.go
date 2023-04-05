/*
Copyright 2023 ztelliot.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type JobConfig struct {
	Upstream        string `json:"upstream"`
	Provider        string `json:"provider"`
	Command         string `json:"command,omitempty"`
	Concurrent      int    `json:"concurrent,omitempty"`
	Interval        int    `json:"interval,omitempty"`
	RsyncOptions    string `json:"rsync_options,omitempty"`
	SizePattern     string `json:"size_pattern,omitempty"`
	AdditionOptions string `json:"addition_options,omitempty"`
}

type DeployConfig struct {
	Image            string                        `json:"image"`
	ImagePullPolicy  corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	NodeName         string                        `json:"nodeName,omitempty"`
	Affinity         *corev1.Affinity              `json:"affinity,omitempty"`
	Tolerations      []corev1.Toleration           `json:"tolerations,omitempty"`
	CPULimit         string                        `json:"cpuLimit,omitempty"`
	MemoryLimit      string                        `json:"memLimit,omitempty"`
}

type PVConfig struct {
	Size         string                            `json:"size"`
	StorageClass *string                           `json:"storageClass,omitempty"`
	AccessMode   corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
}

// JobSpec defines the desired state of Job
type JobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Config JobConfig `json:"config"`

	// +optional
	Deploy DeployConfig `json:"deploy"`

	// +optional
	Volume PVConfig `json:"volume"`
}

type SyncStatus string

const (
	None       SyncStatus = "none"
	Failed     SyncStatus = "failed"
	Success    SyncStatus = "success"
	Syncing    SyncStatus = "syncing"
	PreSyncing SyncStatus = "pre-syncing"
	Paused     SyncStatus = "paused"
	Disabled   SyncStatus = "disabled"
)

type JobStatus struct {
	Status       SyncStatus `json:"status"`
	LastUpdate   int64      `json:"last_update"`
	LastStarted  int64      `json:"last_started"`
	LastEnded    int64      `json:"last_ended"`
	Scheduled    int64      `json:"next_schedule"`
	Upstream     string     `json:"upstream"`
	Size         string     `json:"size"`
	ErrorMsg     string     `json:"error_msg"`
	LastOnline   int64      `json:"last_online"`
	LastRegister int64      `json:"last_register"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Job is the Schema for the jobs API
type Job struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JobSpec   `json:"spec,omitempty"`
	Status JobStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// JobList contains a list of Job
type JobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Job `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Job{}, &JobList{})
}
