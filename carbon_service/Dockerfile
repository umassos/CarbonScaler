# syntax=docker/dockerfile:1

FROM amd64/python:3.9-bullseye

RUN pip3 install numpy pandas flask requests
RUN mkdir /workspace

ADD carbon_service.py /workspace/carbon_service.py
ADD data-sample.csv /workspace/data-sample.csv

WORKDIR /workspace

ENTRYPOINT ["python3", "carbon_service.py"]