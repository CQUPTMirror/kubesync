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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MirrorType string

const (
	Mirror MirrorType = "mirror"
	Proxy  MirrorType = "proxy"
)

type JobConfig struct {
	Alias         string     `json:"alias,omitempty"`
	Desc          string     `json:"desc,omitempty"`
	Url           string     `json:"url,omitempty"`
	HelpUrl       string     `json:"helpUrl,omitempty"`
	Type          MirrorType `json:"type,omitempty"`
	Upstream      string     `json:"upstream"`
	Provider      string     `json:"provider,omitempty"`
	MirrorDir     string     `json:"mirrorDir,omitempty"`
	Command       string     `json:"command,omitempty"`
	Concurrent    int        `json:"concurrent,omitempty"`
	Interval      int        `json:"interval,omitempty"`
	Retry         int        `json:"retry,omitempty"`
	Timeout       int        `json:"timeout,omitempty"`
	FailOnMatch   string     `json:"failOnMatch,omitempty"`
	IPv6Only      string     `json:"IPv6Only,omitempty"`
	IPv4Only      string     `json:"IPv4Only,omitempty"`
	ExcludeFile   string     `json:"excludeFile,omitempty"`
	RsyncOptions  string     `json:"rsyncOptions,omitempty"`
	Stage1Profile string     `json:"stage1Profile,omitempty"`
	ExecOnSuccess string     `json:"execOnSuccess,omitempty"`
	ExecOnFailure string     `json:"execOnFailure,omitempty"`
	SizePattern   string     `json:"sizePattern,omitempty"`
	AdditionEnvs  string     `json:"additionEnvs,omitempty"`
	Debug         string     `json:"debug,omitempty"`
}

type JobDeploy struct {
	DeployConfig `json:",inline"`

	DisableFront string `json:"disableFront,omitempty"`
	FrontImage   string `json:"frontImage,omitempty"`
	FrontCmd     string `json:"frontCmd,omitempty"`
	DisableRsync string `json:"disableRsync,omitempty"`
	RsyncImage   string `json:"rsyncImage,omitempty"`
	RsyncCmd     string `json:"rsyncCmd,omitempty"`
}

type PVConfig struct {
	Size         string                            `json:"size"`
	StorageClass *string                           `json:"storageClass,omitempty"`
	AccessMode   corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
}

// JobSpec defines the desired state of Job
type JobSpec struct {
	Config  JobConfig     `json:"config"`
	Deploy  JobDeploy     `json:"deploy,omitempty"`
	Volume  PVConfig      `json:"volume"`
	Ingress IngressConfig `json:"ingress,omitempty"`
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
	Cached     SyncStatus = "cached"
	Created    SyncStatus = "created"
)

// JobStatus defines the observed state of Job
type JobStatus struct {
	Status       SyncStatus `json:"status"`
	LastUpdate   int64      `json:"lastUpdate"`
	LastStarted  int64      `json:"lastStarted"`
	LastEnded    int64      `json:"lastEnded"`
	Scheduled    int64      `json:"nextSchedule"`
	Upstream     string     `json:"upstream"`
	Size         string     `json:"size"`
	ErrorMsg     string     `json:"errorMsg"`
	LastOnline   int64      `json:"lastOnline"`
	LastRegister int64      `json:"lastRegister"`
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
