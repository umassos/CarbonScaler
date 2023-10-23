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
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Profile struct {
	Throughput         float64
	MarginalThroughput float64
	Power              float64
	MarginalPower      float64
}

func LoadProfile(profile_location string, profile_name string) (map[int]Profile, error) {

	profile_map := make(map[int]Profile)

	profilePath := filepath.Join(profile_location, fmt.Sprintf("%s%s", profile_name, ".csv"))
	readFile, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("Profile Not found")
	}
	csvReader := csv.NewReader(readFile)
	data, err := csvReader.ReadAll()

	if err != nil {
		return nil, errors.New("Cannot Load Profile")
	}

	readFile.Close()
	base_throughput := 0.0
	base_power := 0.0
	for i, line := range data {
		if i > 0 {
			var p Profile
			n := 0
			for j, field := range line {
				if j == 0 {
					n, _ = strconv.Atoi(field)
				} else if j == 1 {
					p.Throughput, _ = strconv.ParseFloat(field, 8)
					p.MarginalThroughput = p.Throughput - base_throughput
					base_throughput = p.Throughput
				} else if j == 2 {
					p.Power, _ = strconv.ParseFloat(field, 8)
					p.MarginalPower = p.Power - base_power
					base_power = p.Power
				}
			}
			profile_map[n] = p
		}
	}

	return profile_map, nil
}
