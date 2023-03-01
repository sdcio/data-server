#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

CLAB_BASE=$SCRIPTPATH/../clab-base/

cd $SCRIPTPATH
sudo CLAB_LABDIR_BASE=${CLAB_BASE} clab destroy -c
