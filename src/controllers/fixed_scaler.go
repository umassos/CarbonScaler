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
	"fmt"
	"os"

	carbonscalerv1 "carbonscaler/api/v1"
	"k8s.io/apimachinery/pkg/types"
)

type FixedScaler struct {
	CarbonStatus CarbonStatus
	ClusterSize  int
	timeSlot     int
}

func (s *FixedScaler) UpdateCarbonStatus(carbonStatus CarbonStatus) error {
	fmt.Fprint(os.Stdout, "Carbon intensity = ", carbonStatus.CarbonIntensity, "\n")
	s.CarbonStatus.CarbonIntensity = carbonStatus.CarbonIntensity
	s.CarbonStatus.CarbonIntensityPrediction = carbonStatus.CarbonIntensityPrediction

	return nil
}

func (s *FixedScaler) GetReplicas(UID types.UID, jobSpec carbonscalerv1.CarbonScalerSpec) (int32, error) {
	replicas := jobSpec.MinReplicas
	return replicas, nil //int32(rand.Intn(4)), nil
}

func (s *FixedScaler) ComputeSchedule(UID types.UID, jobSpec carbonscalerv1.CarbonScalerSpec, work_size float64) error {
	return nil
}
