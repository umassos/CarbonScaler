#!/usr/bin/env python3
import json
from threading import Thread
import threading
import time
from typing import Dict, List
import os
from utils import ElasticJob, get_pods_locations, read_power, get_elastic_jobs, get_power_servers_ips, elastic_job_exists, set_progress
from flask import Flask, request
import argparse
import logging
import requests
import pandas as pd
import math

app = Flask(__name__)

logger = logging.getLogger(__name__)
logger.setLevel(logging.INFO)
logging.basicConfig(format="[%(levelname)s] %(message)s")
CARBON_COST = 0


class JobMonitor(Thread):
    def __init__(self, job: ElasticJob, power_servers: Dict, watch_period: int, logs_location: str) -> None:
        Thread.__init__(self)
        self.job = job
        self.running = True
        self.power_servers = power_servers
        self.watch_period = watch_period
        self.logs_location = logs_location

        self.monitor_name = job.name  # Add other properties
        self.start_time = time.time()
        self.results = []
        self.progress = job.progress
        self.replicas = job.replicas
        self.results.append([self.start_time, self.progress,
                            self.replicas, CARBON_COST, 0, 0, {}])

    def run(self):
        start = time.time()
        while self.running:
            locations = get_pods_locations(self.job.name)
            power = read_power(self.power_servers, locations)
            power = {k: v for k, v in sorted(power.items(), key=lambda item: item[1])}
            for i, (k, v) in enumerate(list(power.items())):
                if i+1 > self.replicas:
                    del power[k]
            assert len(power.items()) <= self.replicas, "Reading error"
            total_power = sum([v for k, v in power.items()])
            real_watch = time.time() - start
            global CARBON_COST
            carbon = total_power * real_watch * \
                CARBON_COST / (1000 * 3600)
            start = time.time()
            self.update_progress()
            set_progress(logger, self.job.name, self.progress)
            logger.info(
                f"{self.job.name}: {locations}, {self.progress}, {total_power}, {carbon}")
            self.results.append(
                [time.time(), self.progress, len(locations), CARBON_COST, total_power, carbon, power])
            time.sleep(self.watch_period)
        self.save_results()

    def update_progress(self):
        try:
            file = os.path.join(self.logs_location,
                                self.job.folder, "progress.csv")
            a_file = open(file, "r")
            last = a_file.readlines()[-1]
            current_progress = math.floor(float(last.split(",")[1]))
            if current_progress > self.progress:
                self.progress = current_progress
        except Exception as e:
            logger.error(f"Update Progress Exception {e}")

    def save_results(self):
        logger.info(
            f"Monitor {self.monitor_name} is done in {time.time() - self.start_time}")
        logs_location = os.path.join(self.logs_location, "logs")
        os.makedirs(logs_location, exist_ok=True)
        df = pd.DataFrame(self.results, columns=[
                          "time", "progress", "replicas", "CarbonCost", "total_power", "carbon", "power_details"])
        file_name = f"{logs_location}/result-{self.monitor_name}.csv"
        logger.info(f"Results saved to {file_name}")
        df.to_csv(file_name)


def run_forever(watch_period, logs_location):
    current_jobs: Dict[str, JobMonitor] = {}
    while True:
        try:
            jobs = get_elastic_jobs()
            power_servers = get_power_servers_ips()
            for job in jobs:
                # new job
                if job.name not in current_jobs and ("Created" in job.conditions or "Paused" in job.conditions) and "Succeeded" not in job.conditions:
                    logger.info(f"Starting Monitoring for job {job.name}")
                    monitor = JobMonitor(job, power_servers,
                                         watch_period, logs_location)
                    print("start monitor")
                    monitor.start()
                    current_jobs[job.name] = monitor
                else:
                    if "Succeeded" in job.conditions and job.name in current_jobs:
                        current_jobs[job.name].running = False
                        del current_jobs[job.name]
                    elif job.name in current_jobs:
                        current_jobs[job.name].replicas = job.replicas
                        #results = current_jobs[job.name].results
            for name, monitor in list(current_jobs.items()):
                if not elastic_job_exists(name):
                    monitor.running = False
                    del current_jobs[name]
        except Exception as e:
            logger.error(f"Loop Exception {e}")
            for name, monitor in list(current_jobs.items()):
                monitor.running = False
                del current_jobs[name]
        time.sleep(watch_period)


def register_simple_status_server(carbon_webhook_address, local_address):
    try:
        data = {
            "app_name": "simple_status_server",
            "address": local_address
        }
        response = requests.post(carbon_webhook_address, json.dumps(data), headers={
                                 "Content-Type": "application/json"})
        if response.status_code == 200:
            logger.info(f"Connecting to Carbon Web Hook is successful.")
            data = json.loads(response.text)
            global CARBON_COST
            CARBON_COST = float(data["carbon_intensity"])
            logger.info(f"Carbon cost is set to {CARBON_COST}")
        else:
            logger.error(f"Connecting to Carbon Web Hook failed.")
    except:
        logger.error(f"Connecting to Carbon Web Hook failed.")
        raise Exception()


@app.route("/", methods=["POST"])
def get_carbon():
    global CARBON_COST
    data = request.json
    CARBON_COST = float(data["carbon_intensity"])
    logger.info(f"Carbon cost is set to {CARBON_COST}")
    return "", 200


def run_carbon_listener(port):
    app.run(host="0.0.0.0", port=port)


def main():
    parser = argparse.ArgumentParser(
        description="Status Server", formatter_class=argparse.ArgumentDefaultsHelpFormatter)
    parser.add_argument("-c", "--carbon-hook", type=str,
                        default="http://172.17.0.5:7777", help="Carbon Web Hook Address")
    parser.add_argument("-a", "--address", type=str,
                        default="http://localhost", help="Status Server address")
    parser.add_argument("-p", "--port", type=int,
                        default=7979, help="Status Server Port")
    parser.add_argument("-w", "--watch-period", type=int,
                        default=5, help="Watch Period Time between readings")
    parser.add_argument("-l", "--logs-location", default=".",
                        type=str, help="Execution Logs locations")

    args = parser.parse_args()

    carbon_listener = threading.Thread(
        target=run_carbon_listener, args=(args.port,))
    carbon_listener.start()

    register_simple_status_server(
            args.carbon_hook, f"{args.address}")
    run_forever(args.watch_period, args.logs_location)


if __name__ == "__main__":
    main()
