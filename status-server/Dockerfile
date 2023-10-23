FROM amd64/python:3.9-bullseye

RUN pip3 install numpy pandas flask requests kubernetes
RUN mkdir /app

ADD status-server.py /app/status-server.py
ADD utils.py /app/utils.py
# The kubernetes configuration is needed to access k8s system
ADD config /app/config
# This next file is only needed for minikube
ADD .minikube /app/.minikube
WORKDIR /app

ENTRYPOINT ["python3", "status-server.py"]