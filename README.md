# CarbonScaler: Leveraging CloudWorkload Elasticity for Optimizing Carbon-Efficiency
This repo presents a hardware-accelerated communication framework for model serving systems.

## Summary
CarbonScaler enables batch applications to decrease their carbon footprint by scaling at low carbon periods and stopping/slowing down at high carbon periods. CarbonScaler is based on a greedy-optimal scheduling policy that yields the minimum carbon footprint.
This repo contains a reference implementation for CarbonScaler. This implementation uses kubernetes to run batch jobs and 
defines a new custom resource definition (CRD) that are based on Kubeflow training operators.
The current implementation uses the job reconcile function to operate in the most carbon-efficient method. The implementation uses:

- golang V1.20
- Kubernetes V1.27
- Kubeflow training operator V1.6.0
- MPI
- Python3

## Paper
The CarbonScaler report is available at [lass website](https://lass.cs.umass.edu/papers/pdf/sigmetrics2024-carbonscaler.pdf). To refer to the paper or the results. Please use the following citation.
```
@article{hanafy2023carbonscaler,
      author={Hanafy, Walid A. and Liang, Qianlin  and Bashir, Noman  and Irwin, David  and Shenoy, Prashant},
      title={{CarbonScaler: Leveraging Cloud Workload Elasticity for Optimizing Carbon-Efficiency}},
      year={2023},
      issue_date = {December 2023},
      volume = {7, 3},
      doi = {10.1145/3626788},
      journal = {Proc. ACM Meas. Anal. Comput. Syst.},
      month = {dec},
      articleno = {57},
      numpages = {28},
}
```

## Project Structure
```bash
tree .
.
├── LICENSE
├── README.md # Instructions 
├── carbon_service # Carbon Intensity Server
├── jobs # Example Job
├── monitoring # Power Monitoring Server
├── setup # Setup auxiliary files
├── src # Source code for CarbonScaler CRD and Manager
└── status-server # Job Monitoring Server
```
## Running CarbonScaler

CarbonScaler was tested on Minikube, Local Cluster, AWS. This repo describes the instructions to run CarbonScaler on Minikube and a Local Cluster.
> You need to have a running version of minikube or K8s before you start. See [minikube](https://minikube.sigs.k8s.io/docs/start/) and [Kubernetes](https://kubernetes.io/docs/setup/) Instructions.


### Step 1: Install kubeflow training operators V1.6
Our code is based on kubeflow training operators that support multiple frameworks to run distributed batch application. We tested CarbonScaler with MPI and Pytorch but it should work with other operators as well. For more info on kubeflow check their [official repo](https://github.com/kubeflow/training-operator).

To install kubeflow
```bash
kubectl apply -k "github.com/kubeflow/training-operator/manifests/overlays/standalone?ref=v1.6.0"
```
Verify its correctly installed by
```bash
> kubectl get crds
NAME                       CREATED AT
mpijobs.kubeflow.org       2023-10-19T20:38:20Z
mxjobs.kubeflow.org        2023-10-19T20:38:21Z
paddlejobs.kubeflow.org    2023-10-19T20:38:21Z
pytorchjobs.kubeflow.org   2023-10-19T20:38:21Z
tfjobs.kubeflow.org        2023-10-19T20:38:21Z
xgboostjobs.kubeflow.org   2023-10-19T20:38:21Z
```

### Step 2: Setup PVC (Persistent Volume Claim)
CarbonScaler needs a persistent storage for maintaining checkpoints and other logs.

#### For Minikube
To create PVC:
```bash
kubectl apply -f setup/minikube-pvc.yaml
```

Make sure its bound

```bash
> kubectl get pvc
NAME            STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
collector-pvc   Bound    pvc-7e683785-acf6-41a4-94d3-4756a7314a51   5Gi        RWX            standard       2s
```
#### For K8s
Please check the instructions in [setup/README.md](setup/README.md)

### Step 3: Run CarbonService
The CarbonService replays the used carbon trace. We utilize trace replay for ensuring reproducibility. However, online carbon services such as [ElectricityMap](https://app.electricitymaps.com/map) or [CarbonCast](https://dl.acm.org/doi/abs/10.1145/3563357.3564079) can be used.

To run the CarbonService.
```bash
kubectl apply -f carbon_service/carbon_service.yaml 
```
> The Docker Image and source code for the Carbon Service is available in the `carbon_service` folder.
### Step 4: Run Power Monitoring API
CarbonScaler need to read the power consumption of the used resources. We implemented a power server that uses RAPL to read CPU power consumption. For GPU power consumption, we are able to utilize [DGCM](https://developer.nvidia.com/blog/monitoring-gpus-in-kubernetes-with-dcgm/). However, polling the power consumption fom GPU using `nvidia-smi` or using an external power meter will work. The Power Monitoring API must be place on each node, we configure this using `replicas:total servers` and `maxSkew: 1`.

To run the power monitoring API.
```
kubectl apply -f monitoring/power_server.yaml
```

> The Docker Image and source code for the Power Monitoring API is available in the `monitoring` folder.

> For testing purposes the Power Monitoring API returns fixed value (10W) if it cannot read the power e.g., ARM and AMD based CPUs that don't support RAPL.

### Step 5: Run the status service
The simple status server implements monitoring and accounting using the k8s python api. The status server is responsible for carbon monitoring and power monitoring. Its connects to the carbon server to get the latest updates. 
The server has an infinite loop for searching for active jobs. If a job is found a new `JobMonitor` is created. This job monitor read the power of the servers where the application has pods. Currently it assumes that the server is pods don't share a server. Once the job is done, its progress is saved.

This container needs to be rebuild with `config` file from your k8s cluster. The `config` file is available in the `.kube` folder. You can rebuild the container and push under your DockeHub ID.
```bash
kubectl apply -f status-server/status-server.yaml
```

> The status server needs information about the cluster. i.e., It needs the master node IP and certificates. You need to rebuild this server for every new deployment.

### Step 6: Build and RUN CarbonScaler Code
CarbonScaler policies is implemented in golang as CRD using kubebuilder templates.  The `src` directory is mostly generated. However our implementation is within:
- api: This holds the data definition needed for the CRDs.
- controllers: This hold the algorithm in (src/controllers/carbon_scaler.go) and the reconcile function, which is called whenever it gets a change notification or when the job is submitted.
- main.go: Starts the schedulers and managers of the new CRD.

To compile the code
```bash
cd src; make
```
Once you see that the code compiles successfully. Follow the deployment instructions in [src/README](src/README.md).
The needed core instructions are:
```bash
make docker-build docker-push IMG=washraf/carbonscaler 
make deploy IMG=washraf/carbonscaler 
```

## Running Jobs
CarbonScaler is a layer on top of Kubeflow training operators. CarbonScaler requires minor additions to the original kubeflow yaml file. This additions are:

```bash
carbonScalerSpec:
    minReplicas: 1 # Minimum Number of nodes (m)
    maxReplicas: 1 # Maximum Number of nodes (M)
    deadline: 1 #Total Time steps
    progress: 0 #Latest Progress, starts with 0 and updated incase of failure.
    profileName: "nbody100k" #profile name from the /src/profiles folder
```
### Example
We provided an example in `jobs/mpi_job.yaml`. The example runs an MPI jobs of the nbody-simulation problem. The code takes number of bodies and simulate their location in 3D space for a certain number of iterations (seconds). The Jobs configurations are provided as environment variable `BODIES`, `ITERATIONS`, and `LOGS_FOLDER`.
To run the example
```bash
kubectrl apply -f jobs/mpi_job.yaml
```

### Configuring the policy
Carbon Scaler behavior depends on the flexibility its given as well as the application profile and carbon intensity. To remove scaling set `minReplicas = maxReplicas`. To remove time shifting set `deadline=JobLengh`. The code also supports a carbon agnostic scheduler that don't use the profile information named `fixed` and is activated by setting `--policy=fixed` for the CarbonScaler manager.

For more details about the policies and their evaluation refer to our paper. 

## CarbonAdvisor
In the paper we proposed CarbonAdvisor, a tool to estimate the carbon footprint on executing batch jobs in the cloud. A basic version [CarbonAdvisor](https://github.com/umassos/carbonAdvisor). We plan to extend it by many features and deploy it soon.

## Contact Information
This code was part of [Walid A. Hanafy](https://people.cs.umass.edu/~whanafy/). Please contact him if you have any questions or issues.

email: whanafy(AT)cs(DOT)umass(DOT)edu

## Limitations
- Independent job scheduling. CarbonScaler assumes jobs are independent and are executed in a single-tenant environment.
