from kubernetes import client, config
from collections import namedtuple
import requests
from typing import Dict, List
import json
import os
from logging import Logger

ElasticJob = namedtuple(
    "ElasticJob", "name start_time folder conditions replicas progress")
if os.path.exists("config"):
    config.load_kube_config(config_file="config")
else:
    config.load_kube_config()
current_client = client.CoreV1Api()


def get_native_jobs(job_name) -> Dict:
    results = client.CustomObjectsApi().list_cluster_custom_object(
        group="kubeflow.org", version="v1", plural="mpijobs")
    for item in results["items"]:
        if item["metadata"]["name"] == job_name:
            return item


def get_elastic_jobs() -> List[ElasticJob]:
    jobs = []
    results = client.CustomObjectsApi().list_cluster_custom_object(
        group="carbonscaler.io", version="v1", plural="carbonscalermpijobs")
    for item in results["items"]:
        native = get_native_jobs(item["metadata"]["name"])

        native_conditions = []
        replicas = 0
        if native and "status" in native:
            native_conditions = [condition["type"] for condition in native["status"]
                                 ["conditions"] if condition["status"] == 'True']
            if "active" in native["status"]["replicaStatuses"]["Worker"]:
                replicas = int(native["status"]
                               ["replicaStatuses"]["Worker"]["active"])

        conditions = [condition["type"] for condition in item["status"]
                      ["conditions"] if condition["status"] == 'True'] + native_conditions
        conditions = list(set(conditions))

        for environment in item["spec"]["mpiReplicaSpecs"]["Launcher"]["template"]["spec"]["containers"][0]["env"]:
            if environment["name"] == "LOGS_FOLDER":
                folder: str = environment["value"]
            elif environment["name"] == "BODIES":
                bodies = environment["value"]

        folder = folder.replace("results/", "")
        folder = os.path.join(folder, bodies)
        if "progress" in item["carbonScalerSpec"]:
            progress = item["carbonScalerSpec"]["progress"]
        else:
            progress = 0
        jobs.append(ElasticJob(
            name=item["metadata"]["name"], start_time=item["metadata"]["creationTimestamp"], folder=folder, conditions=conditions, replicas=replicas,
            progress=progress))
    return jobs


def elastic_job_exists(name: str):
    results = client.CustomObjectsApi().list_cluster_custom_object(
        group="carbonscaler.io", version="v1", plural="carbonscalermpijobs")
    for item in results["items"]:
        if name == item["metadata"]["name"]:
            return True
    return False


def set_progress(logger: Logger, name: str, progress: int):
    results = client.CustomObjectsApi().list_cluster_custom_object(
        group="carbonscaler.io", version="v1", plural="carbonscalermpijobs")
    for item in results["items"]:
        if name == item["metadata"]["name"]:
            if "progress" not in item["carbonScalerSpec"] or item["carbonScalerSpec"]["progress"] != progress:                
                try:
                    item["carbonScalerSpec"]["progress"] = progress
                    client.CustomObjectsApi().patch_namespaced_custom_object(group="carbonscaler.io",
                                                                             version="v1", plural="carbonscalermpijobs", name=name, body=item, namespace="default")
                    logger.info(f"Progress is updated to {progress}")
                except:
                    pass
            return True
    return False


def get_pods_locations(job_name):
    locations = []
    running = False
    ret = current_client.list_pod_for_all_namespaces(watch=False)
    for pod in ret.items:
        if "training.kubeflow.org/replica-type" in pod.metadata.labels and pod.metadata.labels["training.kubeflow.org/replica-type"] == "worker" and pod.metadata.labels["training.kubeflow.org/job-name"]==job_name and pod.status.phase == "Running":
            locations.append(pod.spec.node_name)
        if "training.kubeflow.org/replica-type" in pod.metadata.labels and pod.metadata.labels["training.kubeflow.org/replica-type"] == "launcher" and pod.metadata.labels["training.kubeflow.org/job-name"]==job_name and pod.status.phase == "Running":
            running = True
    if running:
        locations = set(locations)
    else:
        locations = set()
    return locations


def read_power(power_servers, locations):
    all_readings = {}
    if locations == None:
        locations = power_servers.keys()
    for location in locations:
        if location in power_servers:
            response = requests.get(f"http://{power_servers[location]}:6600")
            machine_readings = json.loads(response.text)
            all_readings[location.split(
                ".")[0]] = sum(machine_readings) / len(machine_readings)
        else:
            all_readings[location.split(".")[0]] = 0
    return all_readings


def get_power_servers_ips():
    power_servers = {}
    ret = current_client.list_pod_for_all_namespaces(watch=False)
    for pod in ret.items:
        if pod.metadata.name.startswith("power-server") and pod.status.phase == "Running":
            power_servers[pod.spec.node_name] = pod.status.pod_ip
    return power_servers
