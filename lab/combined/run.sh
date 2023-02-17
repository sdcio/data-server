#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

echo $1
sudo clab des -c

cd ../../
if [[ "$1" == "build" ]];
then
    docker image prune -f
    docker build . -t schema-server:latest
fi

cd $SCRIPTPATH/lab/combined
sudo clab dep -c

cd -
$SCRIPTPATH/../common/configure.sh combined

if [[ "$1" == "build" ]];
then
    docker image prune -f
fi
