/*
Copyright 2023.

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

package v1

import (
	common "github.com/kubeflow/common/pkg/apis/common/v1"
	kfopv1 "github.com/kubeflow/training-operator/pkg/apis/kubeflow.org/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.conditions[-1:].type`
//+kubebuilder:printcolumn:name="Replicas",type=string,JSONPath=`.spec.mpiReplicaSpecs.Worker.replicas`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
//+kubebuilder:printcolumn:name="Progress",type=date,JSONPath=`.carbonScalerSpec.progress`

// CarbonScalerMPIJob is the Schema for the carbonscalermpijobs API
type CarbonScalerMPIJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	CarbonScalerSpec  CarbonScalerSpec  `json:"carbonScalerSpec,omitempty"`
	Spec              kfopv1.MPIJobSpec `json:"spec,omitempty"`
	Status            common.JobStatus  `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CarbonScalerMPIJobList contains a list of CarbonScalerMPIJob
type CarbonScalerMPIJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CarbonScalerMPIJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CarbonScalerMPIJob{}, &CarbonScalerMPIJobList{})
}
