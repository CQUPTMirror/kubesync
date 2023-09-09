/*
Copyright 2023 CQUPTMirror.

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

type DeployConfig struct {
	Image            string                        `json:"image"`
	Env              map[string]string             `json:"env,omitempty"`
	ImagePullPolicy  corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	NodeName         string                        `json:"nodeName,omitempty"`
	Affinity         *corev1.Affinity              `json:"affinity,omitempty"`
	Tolerations      []corev1.Toleration           `json:"tolerations,omitempty"`
	CPULimit         string                        `json:"cpuLimit,omitempty"`
	MemoryLimit      string                        `json:"memLimit,omitempty"`
	ServiceAccount   string                        `json:"serviceAccount,omitempty"`
}

type DeployPhase string

const (
	DeployPending   DeployPhase = "Pending"
	DeploySucceeded DeployPhase = "Succeeded"
	DeployFailed    DeployPhase = "Failed"
)

// ManagerSpec defines the desired state of Manager
type ManagerSpec struct {
	Deploy DeployConfig `json:"deploy"`
}

// ManagerStatus defines the observed state of Manager
type ManagerStatus struct {
	Phase DeployPhase `json:"phase"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Manager is the Schema for the managers API
type Manager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagerSpec   `json:"spec,omitempty"`
	Status ManagerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ManagerList contains a list of Manager
type ManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Manager `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Manager{}, &ManagerList{})
}
