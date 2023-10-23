#!/usr/bin/env python3
"""
    Created date: 3/22/22
"""

import time
import logging
import numpy as np
import pandas as pd
import threading
import sqlite3
import argparse
import requests
import traceback
from concurrent.futures import wait

from flask import Flask, request
from concurrent.futures import ThreadPoolExecutor
from typing import List

DB_NAME = "registration"
DB_PATH = ""

app = Flask(__name__)

logger = logging.getLogger(__name__)
logger.setLevel(logging.INFO)

logging.basicConfig(format="[%(levelname)s] %(message)s")

AGENT = None

def get_db(path: str) -> sqlite3.Connection:
    """ Connect the database """
    db = sqlite3.connect(path)
    return db


def create_table(db: sqlite3.Connection):
    """ Create table if not exists"""
    c = db.cursor()
    sql_cmd = f"""
        CREATE TABLE IF NOT EXISTS {DB_NAME} (app_name TEXT, address TEXT UNIQUE )
    """
    c.execute(sql_cmd)
    db.commit()


class CarbonAgent(threading.Thread):
    """
    An agent to report current and future carbon intensity
    """
    def __init__(self, path: str, update_interval: int,
                 db_path: str, prediction_window: int = 24,
                 percentile_window: int = 24,
                 prediction_method: str = "REAL"):
        """
        Initialize agent
        Args:
            path: path to carbon trace file
            update_interval: update interval in seconds
            db_path: path to user db file
            prediction_window: predicted carbon intensity length
            percentile_window: window compute percentile from
            prediction_method: method to use for carbon prediction

        """
        super(CarbonAgent, self).__init__()
        self._path = path
        self._update_interval = update_interval
        self._db_path = db_path
        self._prediction_window = prediction_window
        self._percentile_window = percentile_window
        self._prediction_method = prediction_method
        self._index_reset = False
        if self._prediction_method == "REAL":
            self.predict_carbon = self.predict_carbon_real
        else:
            # TODO: replace this with other prediction functions
            self.predict_carbon = self.predict_carbon_real
        
        df = pd.read_csv(path)
        # because collected traces are half a second interval
        if "2020" not in path and "2021" not in path :
            df["index"] = range(0, df.shape[0])
            df["index"] //= 2
            df = df.groupby("index").mean().reset_index()
            
        assert "carbon_intensity_avg" in df, "Carbon intensity data not found"
        count_na = df["carbon_intensity_avg"].isna().sum()

        if count_na > 0:
            logger.warning(f"Interpolating {count_na} missing values")
            df.carbon_intensity_avg.interpolate(inplace=True)

        assert df["carbon_intensity_avg"].isna().sum() == 0
        self._carbon_trace = np.array(df["carbon_intensity_avg"], dtype=float)
        self._current_idx = 0
        self._trace_length = self._carbon_trace.shape[0]
        self._running = True
        self._thread_pool = ThreadPoolExecutor(max_workers=5)

        logger.info(f"Loaded trace length: {self._trace_length}")

    def run(self):
        """
        Function to update current carbon intensity every _update_interval,
        this function should be called in a separate thread
        """
        with get_db(self._db_path) as db:
            logger.info(f"Updating carbon every {self._update_interval} seconds")
            while self._running:
                logger.info(f"Updated carbon intensity, idx = {self._current_idx} current value:"
                            f"{self.carbon_intensity}, percentile: {self.carbon_percentile:.4f}")

                c = db.cursor()
                sql_cmd = f"SELECT * FROM {DB_NAME}"
                users = c.execute(sql_cmd).fetchall()
                c.close()

                results = [self._thread_pool.submit(self._send_carbon_update, user) for user in users]
                wait(results)
                # Update and sleep should be done at the end of the iteration.
                for i in range(self._update_interval):
                    # Index set should end current sleep
                    if self._index_reset:
                        break
                    time.sleep(1)
                if self._index_reset:
                    self._index_reset = False
                else:
                    self._current_idx = (self._current_idx + 1) % self._trace_length

    def _send_carbon_update(self, item):
        """Push Updates to the user every time interval.

        Args:
            item (Tuple): (Application Name, Address)
        """
        app_name, address = item
        data = {
            "carbon_intensity": self.carbon_intensity,
            "carbon_percentile": self.carbon_percentile,
            "prediction": self.predict_carbon()
        }

        try:
            r = requests.post(address, json=data)
            logger.info(f"Updating carbon intensity data for app {app_name}, at {address}, "
                        f"code: {r.status_code}")
        except:
            logger.info(f"Fail to connect to {app_name} at {address}")

    def predict_carbon_real(self) -> List:
        """
        Predict the carbon intensity for the next (included current) prediction_window time steps
        Returns:
            predicted_carbon: predicted carbon value

        """
        if self._current_idx + self._prediction_window <= self._trace_length:
            predicted_carbon = \
                self._carbon_trace[self._current_idx:self._current_idx+self._prediction_window]
        else:
            remain_length = self._current_idx + self._prediction_window - self._trace_length
            predicted_carbon = np.concatenate([
                self._carbon_trace[self._current_idx:],
                self._carbon_trace[:remain_length]
            ])

        return [float(v) for v in predicted_carbon]

    def shutdown(self):
        self._running = False

    def set_trace_idx(self, idx) -> bool:
        if 0 <= idx < self._carbon_trace.shape[0]:
            self._current_idx = idx
            self._index_reset = True
            logger.info(f"=> Carbon trace is set to index {idx}")
            return True
        else:
            logger.info(f"=> Fail to set carbon trace index to {idx}, out of range")
            return False

    @property
    def carbon_intensity(self) -> float:
        return self._carbon_trace[self._current_idx]

    @property
    def carbon_percentile(self) -> float:
        if self._current_idx >= self._percentile_window:
            carbon_window = self._carbon_trace[self._current_idx-self._percentile_window:self._current_idx]
        else:
            offset = self._percentile_window - self._current_idx
            carbon_window = np.concatenate([
                self._carbon_trace[-offset:],
                self._carbon_trace[:self._current_idx]
            ])

        return float(np.mean(carbon_window <= self.carbon_intensity))


@app.route("/", methods=["POST"])
def register_address():
    """ Register the client by adding it to the database """
    global AGENT
    data = request.json
    if "app_name" not in data:
        return "app_name not in data", 400
    if "address" not in data:
        return "address not in data", 400

    with get_db(DB_PATH) as db:
        cursor = db.cursor()
        sql_cmd = f"""
        INSERT OR REPLACE INTO {DB_NAME} VALUES 
        ('{data["app_name"]}', '{data["address"]}')  
        """
        cursor.execute(sql_cmd)
        db.commit()

    data = {
        "carbon_intensity": AGENT.carbon_intensity,
        "carbon_percentile": AGENT.carbon_percentile,
        "prediction": AGENT.predict_carbon()
    }

    return data, 200


@app.route("/set_index", methods=["POST"])
def set_carbon_trace_idx():
    """ Set carbon trace to a specified index """
    global AGENT
    data = request.values

    try:
        idx = int(data["idx"])
        if not AGENT.set_trace_idx(idx):
            raise ValueError(f"set carbon trace index fail, bad value {idx}")
    except:
        return traceback.format_exc(), 400

    return f"Carbon trace index is set to {idx} successfully", 200


def main():
    parser = argparse.ArgumentParser(description="Carbon Agent Arguments", formatter_class=argparse.ArgumentDefaultsHelpFormatter)
    parser.add_argument(
        "-f",
        "--carbon-path",
        metavar="FILEPATH",
        dest="filepath",
        required=True,
        type=str,
        help="Path to carbon trace file"
    )
    
    parser.add_argument(
        "-d",
        "--db-path",
        metavar="DBPATH",
        dest="db_path",
        required=True,
        type=str,
        help="Path to .db SQLite database path"
    )
    
    parser.add_argument(
        "-i",
        "--update-interval",
        metavar="SECONDS",
        dest="update_interval",
        default=30,
        type=int,
        help="Update interval for carbon values"
    )
    
    parser.add_argument(
        "-w",
        "--prediction-window",
        metavar="TIMESTEPS",
        dest="prediction_window",
        default=24,
        type=int,
        help="Prediction window length of carbon agent"
    )

    parser.add_argument(
        "-p",
        "--percentile-window",
        metavar="TIMESTEPS",
        dest="percentile_window",
        default=24,
        type=int,
        help="Percentile window length of carbon agent"
    )
    
    parser.add_argument("-m",
                        "--prediction-method",
                        default="REAL",
                        choices=["REAL", "PRED"],
                        type=str, help="Carbon Prediction Method")
    args = parser.parse_args()

    global DB_PATH
    DB_PATH = args.db_path

    with get_db(args.db_path) as db:
        create_table(db)

    global AGENT
    AGENT = CarbonAgent(args.filepath, args.update_interval, args.db_path, args.prediction_window,
                        args.percentile_window, args.prediction_method)
    AGENT.start()

    try:
        app.run(host="0.0.0.0", port="7777")
    finally:
        AGENT.shutdown()
        AGENT.join()


if __name__ == '__main__':
    main()