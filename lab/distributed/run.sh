#!/bin/bash

sudo clab dep -c

docker image prune -f
cd ../../
docker build . -t schema-server:latest

cd ../common
./configure.sh distributed
docker image prune -f