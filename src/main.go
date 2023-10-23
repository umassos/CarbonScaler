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

package main

import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/kubeflow/common/pkg/controller.v1/common"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	carbonscalerv1 "carbonscaler/api/v1"
	"carbonscaler/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(carbonscalerv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var carbonWebhookPort int
	var carbonServerAddr string
	var controllerAddr string
	var time_slot int
	var policy string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&carbonWebhookPort, "carbon-webhook-port", 8899, "Port to receive carbon updates")
	flag.StringVar(&controllerAddr, "controller-address", "http://carbonscaler-controller-manager", "Controller Address")
	flag.StringVar(&carbonServerAddr, "carbon-server-address", "http://carbon-service-svc", "CarbonService Address")
	flag.StringVar(&policy, "policy", "scaler", "Scheduling Policy")
	flag.IntVar(&time_slot, "time_slot", 300, "Simulated Time Slot")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "6a2e7784.carbonscaler.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	gangSchedulingSetupFunc := common.GenNonGangSchedulerSetupFunc()
	reconciler, err := controllers.NewReconciler(mgr, gangSchedulingSetupFunc, "", "")
	if err != nil {
		setupLog.Error(err, "Unable to create reconciler")
		os.Exit(1)
	}
	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CarbonElasticMPIhJob")
		os.Exit(1)
	}

	autoScaler := controllers.NewScaler(policy, time_slot, 8)
	if autoScaler == nil {
		setupLog.Error(err, "Unable to create a scaler")
		os.Exit(1)
	}
	//Application Controllers are a list of supported reconcilers
	var ApplicationControllers []controllers.ApplicationController
	ApplicationControllers = append(ApplicationControllers, reconciler)
	reconciler.AutoScaler = autoScaler

	carbonWebhook := controllers.CarbonWebhook{
		ApplicationControllers: ApplicationControllers,
		AutoScaler:             autoScaler,
	}
	if err := carbonWebhook.Register(mgr, carbonWebhookPort, controllerAddr, carbonServerAddr); err != nil {
		setupLog.Error(err, "Unable to register webhook")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
