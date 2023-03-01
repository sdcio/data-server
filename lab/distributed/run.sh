#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

CLAB_BASE=$SCRIPTPATH/../clab-base/

ACTUALDIR=$(pwd)

cd $SCRIPTPATH
sudo CLAB_LABDIR_BASE=${CLAB_BASE} clab destroy -t distributed.clab.yaml -c

if [[ "$1" == "build" ]];
then
    cd ../../
    docker image prune -f
    docker build . -t schema-server:latest
fi

cd $SCRIPTPATH
sudo CLAB_LABDIR_BASE=${CLAB_BASE} clab deploy -t distributed.clab.yaml -c

$SCRIPTPATH/../common/configure.sh distributed

if [[ "$1" == "build" ]];
then
    docker image prune -f
fi

cd ${ACTUALDIR}