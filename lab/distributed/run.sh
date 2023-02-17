#!/bin/bash

sudo clab des -t distributed.clab.yaml -c

docker image prune -f
cd ../../
if [[ "$1" == "build" ]];
then
    docker image prune -f
    docker build . -t schema-server:latest
fi

cd lab/distributed/
sudo clab dep -t distributed.clab.yaml -c

# cd ../common
# ./configure.sh distributed

if [[ "$1" == "build" ]];
then
    docker image prune -f
fi

