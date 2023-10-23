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
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"

	carbonscalerv1 "carbonscaler/api/v1"

	"k8s.io/apimachinery/pkg/types"
)

type CarbonScaler struct {
	CarbonStatus  CarbonStatus
	ClusterSize   int
	scheduleIndex map[types.UID]int32
	schedule      map[types.UID][]int32
	slock         sync.Mutex
	timeSlot      int
}

type mar_cap_carbon struct {
	nodes int32
	time  int
	value float64
}

func (s *CarbonScaler) UpdateCarbonStatus(carbonStatus CarbonStatus) error {
	fmt.Fprint(os.Stdout, "Carbon intensity = ", carbonStatus.CarbonIntensity, "\n")
	s.CarbonStatus.CarbonIntensity = carbonStatus.CarbonIntensity
	s.CarbonStatus.CarbonIntensityPrediction = carbonStatus.CarbonIntensityPrediction

	return nil
}

func (s *CarbonScaler) GetReplicas(UID types.UID, jobSpec carbonscalerv1.CarbonScalerSpec) (int32, error) {
	s.slock.Lock()
	defer s.slock.Unlock()
	//We have Two cases
	// Case 1: Schedule Exist
	if schedule, ok := s.schedule[UID]; ok {
		index := s.scheduleIndex[UID]
		return schedule[index], nil
	}
	// Case 2: Schedule Doesn't Exist
	return 0, nil //int32(rand.Intn(4)), nil
}

func (s *CarbonScaler) ComputeSchedule(UID types.UID, jobSpec carbonscalerv1.CarbonScalerSpec, work_size float64) error {
	s.slock.Lock()
	defer s.slock.Unlock()
	//We have three case
	// Case 1 and 2 Schedule Exist
	if _, ok := s.schedule[UID]; ok {
		// Case 1 Schedule exist and it was fine
		if int(s.scheduleIndex[UID]+1) < len(s.schedule[UID]) {
			s.scheduleIndex[UID] += 1
		} else {
			// Case 2 Schedule exist and it was not enough
			s.schedule[UID] = []int32{jobSpec.MinReplicas}
			s.scheduleIndex[UID] = 0
		}

		return nil
	}
	//Case 3 schedule donn't exist
	// Load profiles
	profile_map, err := LoadProfile("profiles", jobSpec.ProfileName)
	if err != nil {
		fmt.Fprint(os.Stdout, "Cannot Load Profile")
		return err
	}
	//Max Nodes is set by the cluster and application

	max_nodes := int(math.Min(float64(s.ClusterSize), float64(jobSpec.MaxReplicas)))
	mar_cap_carbon_list := make([]mar_cap_carbon, 0)

	//todo: handle the case of restarted job or schedule changed

	// collect marginal capacity per carbon
	for i := 0; i < int(jobSpec.DeadLine); i++ {
		for j := int(jobSpec.MinReplicas); j <= max_nodes; j++ {
			value := profile_map[j].MarginalThroughput / (profile_map[j].MarginalPower * s.CarbonStatus.CarbonIntensityPrediction[i])
			mar_cap_carbon_list = append(mar_cap_carbon_list, mar_cap_carbon{
				nodes: int32(j),
				time:  i,
				value: value,
			})
		}
	}
	//sort them according to marginal capacity per carbon
	sort.Slice(mar_cap_carbon_list, func(i, j int) bool {
		return mar_cap_carbon_list[i].value > mar_cap_carbon_list[j].value
	})

	schedule := make([]int32, jobSpec.DeadLine)
	remaining_work := (1.0 - float64(jobSpec.Progress)/100.0) * work_size

	//Iterate over options until the schedule is computed
	done := 0.0
	for done < remaining_work {
		index := -1
		for i, item := range mar_cap_carbon_list {
			if item.nodes == jobSpec.MinReplicas {
				schedule[item.time] = int32(item.nodes)
				index = i
				break
			} else if item.nodes == schedule[item.time]+1 {
				schedule[item.time] = int32(item.nodes)
				index = i
				break
			}
		}
		if index == -1 {
			fmt.Fprint(os.Stdout, "Deadline too tight")
			return errors.New("cannot compute schedule")
		}
		//remove the used index
		mar_cap_carbon_list = append(mar_cap_carbon_list[:index], mar_cap_carbon_list[index+1:]...)

		//compute the expected done work
		done = compute_done(schedule, profile_map, float64(s.timeSlot))
	}

	fmt.Fprint(os.Stdout, "\n******The Schedule is computed correctly. \n")
	fmt.Fprint(os.Stdout, "S = ", schedule, "\n")
	s.schedule[UID] = schedule
	s.scheduleIndex[UID] = 0
	return nil
}

func compute_done(schedule []int32, profile_map map[int]Profile, time_slot float64) float64 {
	done := 0.0
	for _, v := range schedule {
		done = done + profile_map[int(v)].Throughput*time_slot
	}
	return done
}
