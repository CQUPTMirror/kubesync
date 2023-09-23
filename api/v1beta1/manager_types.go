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
}

type DeployPhase string

const (
	DeployPending   DeployPhase = "Pending"
	DeploySucceeded DeployPhase = "Succeeded"
	DeployFailed    DeployPhase = "Failed"
)

// ManagerSpec defines the desired state of Manager
type ManagerSpec struct {
	Deploy DeployConfig `json:",inline"`
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
