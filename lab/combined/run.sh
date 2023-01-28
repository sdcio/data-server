#!/bin/bash

echo $1
sudo clab des -c

cd ../../
if [[ "$1" == "build" ]];
then
    docker image prune -f
    docker build . -t schema-server:latest
fi

cd lab/combined
sudo clab dep -c
cd ../common
./configure.sh combined
if [[ "$1" == "build" ]];
then
    docker image prune -f
fi
