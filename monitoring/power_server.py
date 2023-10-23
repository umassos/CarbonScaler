from flask import Flask
import logging
from rapl import PowerMeter
from flask import jsonify
app = Flask(__name__)
logger = logging.getLogger(__name__)
logger.setLevel(logging.INFO)
logging.basicConfig(format="[%(levelname)s] %(message)s")
METER = None

@app.route("/")
def getpower():
    global METER
    readings = METER.get_readings()
    return jsonify(readings), 200

def main():
    global METER
    METER = PowerMeter(1)
    METER.start()

    app.run(host="0.0.0.0", port=6600, debug=True)
    
if __name__ == "__main__":
    main()