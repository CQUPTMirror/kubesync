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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AnnouncementSpec defines the desired state of Announcement
type AnnouncementSpec struct {
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
}

// AnnouncementStatus defines the observed state of Announcement
type AnnouncementStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Announcement is the Schema for the announcements API
type Announcement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AnnouncementSpec   `json:"spec,omitempty"`
	Status AnnouncementStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AnnouncementList contains a list of Announcement
type AnnouncementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Announcement `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Announcement{}, &AnnouncementList{})
}
