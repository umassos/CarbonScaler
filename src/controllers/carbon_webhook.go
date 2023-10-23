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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	ctrl "sigs.k8s.io/controller-runtime"
)

type CarbonWebhook struct {
	mux  *http.ServeMux
	addr string
	AutoScaler
	ApplicationControllers []ApplicationController
}

type AppConfig struct {
	AppName string `json:"app_name"`
	Address string `json:"address"`
}

type CarbonStatus struct {
	CarbonIntensity           float64   `json:"carbon_intensity"`
	CarbonIntensityPrediction []float64 `json:"prediction"`
}

func (c *CarbonWebhook) Start(ctx context.Context) error {
	srv := http.Server{
		Addr:    c.addr,
		Handler: c.mux,
	}

	idleConnsClosed := make(chan struct{})

	go func() {
		<-ctx.Done()
		fmt.Println("Shutting down Carbon Webhook")
		err := srv.Shutdown(context.Background())
		if err != nil {
			fmt.Println(err)
		}
		close(idleConnsClosed)
	}()

	err := srv.ListenAndServe()
	if err != nil {
		return err
	}

	<-idleConnsClosed
	return err
}

func (c CarbonWebhook) Register(mgr ctrl.Manager, carbonWebhookPort int, controllerAddr string, carbonServerAddr string) error {
	if err := mgr.Add(&c); err != nil {
		return err
	}

	c.mux = http.NewServeMux()
	c.mux.Handle("/", &c)
	c.addr = fmt.Sprintf(":%d", carbonWebhookPort)

	config := AppConfig{
		AppName: "CarbonScaler",
		Address: controllerAddr,
	}

	jsonValues, _ := json.Marshal(config)
	res, err := http.Post(carbonServerAddr, "application/json", bytes.NewBuffer(jsonValues))
	if err != nil {
		return err
	}

	if res == nil {
		fmt.Println("Error")
	} else if res.StatusCode == http.StatusOK {
		var carbonStatus CarbonStatus
		decoder := json.NewDecoder(res.Body)
		if err := decoder.Decode(&carbonStatus); err != nil {
			return err
		}

		if err := c.AutoScaler.UpdateCarbonStatus(carbonStatus); err != nil {
			return err
		}
	} else {
		return err
	}
	return nil
}

// This function service the http calls from the carbon web hook service
func (c CarbonWebhook) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusBadRequest)
		_, err := writer.Write([]byte("Only accept POST request"))
		if err != nil {
			fmt.Println("Write fail ")
		}

		return
	}
	//Load Carbon Status from Request
	var carbonStatus CarbonStatus
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(&carbonStatus); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := c.AutoScaler.UpdateCarbonStatus(carbonStatus); err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.WriteHeader(http.StatusOK)
	//Schedule All Jobs in all applications after return
	go func() {
		for _, applicationController := range c.ApplicationControllers {
			if err := applicationController.ScheduleJobs(); err != nil {
				fmt.Println("ScheduleJobs error")

			}
		}
	}()
}
