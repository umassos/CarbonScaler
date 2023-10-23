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
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	commonv1 "github.com/kubeflow/common/pkg/apis/common/v1"
	"github.com/kubeflow/common/pkg/controller.v1/common"
	commonutil "github.com/kubeflow/common/pkg/util"
	kfopv1 "github.com/kubeflow/training-operator/pkg/apis/kubeflow.org/v1"
	trainingoperatorcommon "github.com/kubeflow/training-operator/pkg/common"
	"github.com/kubeflow/training-operator/pkg/common/util"
	"github.com/kubeflow/training-operator/pkg/controller.v1/mpi"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	carbonscalerv1 "carbonscaler/api/v1"
)

const (
	// JobPaused means the job is stopped intentionally
	JobPaused       commonv1.JobConditionType = "Paused"
	UndefinedStatus commonv1.JobConditionType = "Undefined"
	// ControllerName is the name of this controller
	ControllerName = "CarbonScalerMPIJobController"

	// JobPausedReason is added in a job when it is paused.
	JobPausedReason = "JobStopped"
	UndefinedReason = "No Conditions"
)

// CarbonScalerMPIJobReconciler reconciles a CarbonScalerMPIJob object
type CarbonScalerMPIJobReconciler struct {
	CarbonScalerReconciler
	mpi.MPIJobReconciler
	AutoScaler
}

//+kubebuilder:rbac:groups=carbonscaler.io,resources=carbonscalermpijobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=carbonscaler.io,resources=carbonscalermpijobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=carbonscaler.io,resources=carbonscalermpijobs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CarbonScalerMPIJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *CarbonScalerMPIJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	logger := r.Log.WithValues("CarbonScalerMPIJob", req.NamespacedName)

	scalerJob := &carbonscalerv1.CarbonScalerMPIJob{}

	err := r.Get(ctx, req.NamespacedName, scalerJob)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If job has succeeded or failed, no need to reconcile.
	if commonutil.IsSucceeded(scalerJob.Status) || commonutil.IsFailed(scalerJob.Status) {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling Job")
	workerReplicas := scalerJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeWorker].Replicas
	launcherReplicas := scalerJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeLauncher].Replicas
	// If number replicas is not specified, set using AutoScaler
	scaleResult, e := r.AutoScaler.GetReplicas(scalerJob.UID, scalerJob.CarbonScalerSpec)
	if e != nil {
		return ctrl.Result{}, e
	} else {
		*workerReplicas = scaleResult
	}

	if *workerReplicas == 0 {
		*launcherReplicas = 0
	} else {
		*launcherReplicas = 1
	}

	//Update the original definition (in case of mismatch at the begining)
	scalerJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeWorker].Replicas = workerReplicas
	scalerJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeLauncher].Replicas = launcherReplicas
	for idx, env := range scalerJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeLauncher].Template.Spec.Containers[0].Env {
		if env.Name == "WORKERS" {
			current_replicas, _ := strconv.Atoi(env.Value)
			if current_replicas != int(*workerReplicas) {
				scalerJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeLauncher].Template.Spec.Containers[0].Env[idx].Value = strconv.Itoa(int(*workerReplicas))
			}
		}
	}

	shouldStop := *workerReplicas == 0
	shouldRestartLaucher := false
	baseJob := &kfopv1.MPIJob{}
	err = r.Get(ctx, req.NamespacedName, baseJob)
	shouldCreate := err != nil

	if shouldCreate && !shouldStop {
		logger.Info("MPIJob Not found, creating one.")

		constructMPIJob := func(scalerJob *carbonscalerv1.CarbonScalerMPIJob,
			baseJob *kfopv1.MPIJob) error {
			jobSpec := *scalerJob.Spec.DeepCopy()

			baseJob.Kind = kfopv1.MPIJobKind
			baseJob.APIVersion = kfopv1.GroupVersion.String()
			baseJob.ObjectMeta = metav1.ObjectMeta{
				Name:      scalerJob.Name,
				Namespace: scalerJob.Namespace,
			}
			baseJob.Spec = jobSpec

			createdCondition := commonv1.JobCondition{
				Type:   commonv1.JobCreated,
				Status: "True",
			}

			baseJob.Status = commonv1.JobStatus{
				Conditions: []commonv1.JobCondition{createdCondition},
			}

			if e := ctrl.SetControllerReference(scalerJob, baseJob, r.Scheme); e != nil {
				logger.Error(e, "Unable to set MPIJob owner")
				return e
			}

			return nil
		}

		if e := constructMPIJob(scalerJob, baseJob); e != nil {
			logger.Error(e, "Unable to create MPIJob spec")
			return ctrl.Result{}, e
		}
		if e := r.Create(ctx, baseJob); e != nil {
			logger.Error(e, "Unable to create MPIJob")
			return ctrl.Result{}, e
		}
	} else if shouldStop && !shouldCreate {
		logger.Info("Stopping job...")
		if err := r.Delete(ctx, baseJob); err != nil {
			return ctrl.Result{}, err
		}
	} else if !shouldCreate {
		logger.Info("Updating job")
		baseJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeWorker].Replicas = workerReplicas
		for idx, env := range baseJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeLauncher].Template.Spec.Containers[0].Env {
			if env.Name == "WORKERS" {
				current_replicas, _ := strconv.Atoi(env.Value)
				if current_replicas != int(*workerReplicas) {
					shouldRestartLaucher = true
					baseJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeLauncher].Template.Spec.Containers[0].Env[idx].Value = strconv.Itoa(int(*workerReplicas))
				}
			}
		}
		trainingoperatorcommon.ClearGeneratedFields(&scalerJob.ObjectMeta)
		if err := r.Update(ctx, baseJob); err != nil {
			return ctrl.Result{}, err
		}
	}

	if shouldRestartLaucher {
		logger.Info("shouldRestartLaucher job")
		//podt := &corev1.Pod{}
		//r.MPIJobReconciler.DeleteAllOf(ctx, podt)
		//r.MPIJobReconciler.DeleteJob(mpiJob)
		//err = r.MPIJobReconciler.KubeClientSet.CoreV1().Pods(req.NamespacedName).Delete(context.Background(), "nbody-launcher", metav1.DeleteOptions{})
		//if err != nil {
		//	return ctrl.Result{}, nil
		//}
		err = r.deleteLauncher(baseJob)
		if err != nil {
			logger.Error(err, "Reconcile CarbonScalerMPIJob error")
			return ctrl.Result{}, nil
		}

	}

	err = r.UpdateJobStatus(scalerJob, baseJob, shouldStop)
	if err != nil {
		logger.Error(err, "Reconcile CarbonScalerMPIJob error")
		return ctrl.Result{}, nil
	}

	if err := r.UpdateJobStatusInApiServer(scalerJob, &scalerJob.Status); err != nil {
		logger.Error(err, "Update job status in API server fail")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// UpdateJobStatus updates the job status and job conditions
func (r *CarbonScalerMPIJobReconciler) UpdateJobStatus(scalerJob *carbonscalerv1.CarbonScalerMPIJob,
	baseJob *kfopv1.MPIJob, shouldStop bool) error {

	scalerJobKey, err := common.KeyFunc(scalerJob)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for CarbonScalerMPIJob object %#v: %v", scalerJob, err))
		return err
	}

	logger := commonutil.LoggerForJob(scalerJob)

	carbonJobStatus := &scalerJob.Status
	//baseJobStatus := baseJob.Status

	// Set StartTime.
	if carbonJobStatus.StartTime == nil {
		now := metav1.Now()
		carbonJobStatus.StartTime = &now
		// enqueue a sync to check if job past ActiveDeadlineSeconds
		if scalerJob.Spec.RunPolicy.ActiveDeadlineSeconds != nil {
			logger.Infof("Job with ActiveDeadlineSeconds will sync after %d seconds", *scalerJob.Spec.RunPolicy.ActiveDeadlineSeconds)
			r.WorkQueue.AddAfter(scalerJobKey, time.Duration(*scalerJob.Spec.RunPolicy.ActiveDeadlineSeconds)*time.Second)
		}
	}

	carbonJobStatus.Conditions = []commonv1.JobCondition{}
	if shouldStop {
		msg := fmt.Sprintf("CarbonScalerMPIJob stopped.")

		err := commonutil.UpdateJobConditions(carbonJobStatus, JobPaused, JobPausedReason, msg)
		if err != nil {
			commonutil.LoggerForJob(scalerJob).Infof("Append CarbonScalerMPIJob error %v", err)
			return err
		}
	} else {
		if len(baseJob.Status.Conditions) == 0 {
			//This trick is used to add a fake status when the job didn't have a status when the function was called.
			msg := fmt.Sprintf("CarbonScalerMPIJob cannot find Conditions.")

			err := commonutil.UpdateJobConditions(carbonJobStatus, UndefinedStatus, UndefinedReason, msg)
			if err != nil {
				commonutil.LoggerForJob(scalerJob).Infof("Append CarbonScalerMPIJob error %v", err)
				return err
			}
		}
		for _, cond := range baseJob.Status.Conditions {
			msg := fmt.Sprintf("CarbonScalerMPIJob %v.", cond.Type)
			err := commonutil.UpdateJobConditions(carbonJobStatus, cond.Type, "Updated based on MPIJob", msg)
			if err != nil {
				commonutil.LoggerForJob(scalerJob).Infof("Append CarbonScalerMPIJob error %v", err)
				return err
			}
		}
	}

	return nil
}

func (r *CarbonScalerMPIJobReconciler) deleteLauncher(baseJob *kfopv1.MPIJob) error {
	podlist := &corev1.PodList{}
	err := r.MPIJobReconciler.List(context.Background(), podlist, client.InNamespace(baseJob.GetNamespace()))

	if err != nil {
		return err
	}
	for _, pod := range podlist.Items {
		if strings.Contains(pod.Name, "launcher") {
			err = r.MPIJobReconciler.KubeClientSet.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil

}

// UpdateJobStatusInApiServer updates the job status in to cluster.
func (r *CarbonScalerMPIJobReconciler) UpdateJobStatusInApiServer(job interface{}, jobStatus *commonv1.JobStatus) error {
	if jobStatus.ReplicaStatuses == nil {
		jobStatus.ReplicaStatuses = map[commonv1.ReplicaType]*commonv1.ReplicaStatus{}
	}

	scalerJob, ok := job.(*carbonscalerv1.CarbonScalerMPIJob)
	//trainingoperatorcommon.ClearGeneratedFields(&scalerJob.ObjectMeta)
	if !ok {
		return fmt.Errorf("%+v is not a type of CarbonScalerMPIJob", job)
	}

	// Job status passed in differs with status in job, update in basis of the passed in one.
	if !equality.Semantic.DeepEqual(&scalerJob.Status, jobStatus) {
		scalerJob = scalerJob.DeepCopy()
		scalerJob.Status = *jobStatus.DeepCopy()
	}

	result := r.Status().Update(context.Background(), scalerJob)

	if result != nil {
		r.Log.WithValues("CarbonScalerMPIJob", types.NamespacedName{
			Namespace: scalerJob.GetNamespace(),
			Name:      scalerJob.GetName(),
		}).Error(result, "update fail")
		return result
	}

	return nil
}

// GetPodsForJob Always return empty pod list as CarbonScalerMPIJobReconciler does not control Pod directly
func (r *CarbonScalerMPIJobReconciler) GetPodsForJob(obj interface{}) ([]*corev1.Pod, error) {
	podlist := &corev1.PodList{}
	return util.ConvertPodList(podlist.Items), nil
}

// GetServicesForJob Always return empty pod list as CarbonScalerMPIJobReconciler does not control Pod directly
func (r *CarbonScalerMPIJobReconciler) GetServicesForJob(obj interface{}) ([]*corev1.Service, error) {
	svcList := &corev1.ServiceList{}
	return util.ConvertServiceList(svcList.Items), nil
}

// ReconcilePods Always return nil as CarbonScalerMPIJobReconciler does not control Pod directly
func (r *CarbonScalerMPIJobReconciler) ReconcilePods(
	job interface{},
	jobStatus *commonv1.JobStatus,
	pods []*corev1.Pod,
	rtype commonv1.ReplicaType,
	spec *commonv1.ReplicaSpec,
	replicas map[commonv1.ReplicaType]*commonv1.ReplicaSpec,
) error {
	return nil
}

// ReconcileServices is overridden because CarbonScalerMPIJobReconciler does not need to reconcile services
func (jc *CarbonScalerMPIJobReconciler) ReconcileServices(
	job metav1.Object,
	services []*corev1.Service,
	rtype commonv1.ReplicaType,
	spec *commonv1.ReplicaSpec) error {
	return nil
}

func NewReconciler(mgr manager.Manager, gangSchedulingSetupFunc common.GangSchedulingSetupFunc, scaler_type string, profileLocation string) (*CarbonScalerMPIJobReconciler, error) {
	r := &CarbonScalerMPIJobReconciler{
		MPIJobReconciler:       *mpi.NewReconciler(mgr, gangSchedulingSetupFunc),
		CarbonScalerReconciler: CarbonScalerReconciler{},
	}
	r.CarbonScalerReconciler.ProfileLocation = profileLocation
	return r, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CarbonScalerMPIJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	utilruntime.Must(kfopv1.AddToScheme(mgr.GetScheme()))
	return ctrl.NewControllerManagedBy(mgr).
		For(&carbonscalerv1.CarbonScalerMPIJob{}).
		Complete(r)
}

func (r *CarbonScalerMPIJobReconciler) ScheduleJobs() error {
	carbonJobList := carbonscalerv1.CarbonScalerMPIJobList{}
	if err := r.List(context.Background(), &carbonJobList); err != nil {
		fmt.Println("Get JobList error")
		return nil
	}

	for _, scalerJob := range carbonJobList.Items {
		if !commonutil.IsSucceeded(scalerJob.Status) && !commonutil.IsFailed(scalerJob.Status) {
			if err := r.ScheduleJob(scalerJob); err != nil {
				fmt.Println(err)
			}
		}
	}
	return nil
}

func (r *CarbonScalerMPIJobReconciler) ScheduleJob(scalerJob carbonscalerv1.CarbonScalerMPIJob) error {
	bodies, iterations := 0, 0
	for _, env := range scalerJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeLauncher].Template.Spec.Containers[0].Env {
		if env.Name == "BODIES" {
			bodies, _ = strconv.Atoi(env.Value)
		} else if env.Name == "ITERATIONS" {
			iterations, _ = strconv.Atoi(env.Value)
		}
	}
	err := r.AutoScaler.ComputeSchedule(scalerJob.UID, scalerJob.CarbonScalerSpec, float64(bodies*iterations))
	if err != nil {
		return fmt.Errorf("Cannot Compute Schedule of job %+v", scalerJob)
	}
	replicas, err := r.AutoScaler.GetReplicas(scalerJob.UID, scalerJob.CarbonScalerSpec)
	if err != nil {
		return fmt.Errorf("cannot get replica of job %+v", scalerJob)
	}
	scalerJob.Spec.MPIReplicaSpecs[kfopv1.MPIJobReplicaTypeWorker].Replicas = &replicas
	if err := r.Update(context.Background(), &scalerJob); err != nil {
		return fmt.Errorf("update fail for job %+v", scalerJob)
	}

	return nil
}
