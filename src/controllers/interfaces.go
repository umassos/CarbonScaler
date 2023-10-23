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

package controllers

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"

	carbonscalerv1 "carbonscaler/api/v1"
)

type ApplicationController interface {
	// ScheduleJobs
	ScheduleJobs() error
}

type AutoScaler interface {
	// UpdateCarbonStatus updates the metrics used to make scaling decision
	UpdateCarbonStatus(CarbonStatus) error

	// GetReplicas returns the number of replicas to run for given job
	GetReplicas(types.UID, carbonscalerv1.CarbonScalerSpec) (int32, error)

	// Compute Schedule for a job
	ComputeSchedule(types.UID, carbonscalerv1.CarbonScalerSpec, float64) error
}

func NewScaler(scaler_type string, timeSlot int, cluster_size int) AutoScaler {
	if scaler_type == "fixed" {
		return &FixedScaler{
			CarbonStatus: CarbonStatus{},
			ClusterSize:  cluster_size,
			timeSlot:     timeSlot,
		}
	}
	if scaler_type == "scaler" {
		return &CarbonScaler{
			CarbonStatus:  CarbonStatus{},
			ClusterSize:   cluster_size,
			timeSlot:      timeSlot,
			scheduleIndex: make(map[types.UID]int32),
			schedule:      make(map[types.UID][]int32),
			slock:         sync.Mutex{},
		}
	}
	return nil
}
