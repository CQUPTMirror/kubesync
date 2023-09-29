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

type FileType string

const (
	OS  FileType = "os"
	App FileType = "app"
)

type FileInfo struct {
	Name         string `json:"name"`
	Ext          string `json:"ext"`
	MajorVersion string `json:"majorVersion"`
	Version      string `json:"version"`
	Arch         string `json:"arch"`
	Edition      string `json:"edition"`
	EditionType  string `json:"editionType"`
	Part         int    `json:"part"`
	Path         string `json:"path"`
}

// FileSpec defines the desired state of File
type FileSpec struct {
	Type  FileType `json:"type,omitempty"`
	Alias string   `json:"alias,omitempty"`
}

// FileStatus defines the observed state of File
type FileStatus struct {
	Files      []FileInfo `json:"files"`
	UpdateTime int64      `json:"updateTime"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// File is the Schema for the files API
type File struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileSpec   `json:"spec,omitempty"`
	Status FileStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FileList contains a list of File
type FileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []File `json:"items"`
}

func init() {
	SchemeBuilder.Register(&File{}, &FileList{})
}
