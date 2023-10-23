# based on https://github.com/spagnuolocarmine/docker-mpi
FROM ubuntu:18.04

RUN apt-get --yes -qq update \
 && apt-get --yes -qq upgrade \
 && apt-get --yes -qq install \
                      python3 \
                      python3-pip \ 
 && apt-get --yes -qq clean \
 && rm -rf /var/lib/apt/lists/*

RUN pip3 install flask
WORKDIR /power_server

COPY power_server.py .
COPY rapl.py .

CMD [ "python3", "power_server.py" ]



