#!/bin/bash
# Copyright 2024 Nokia
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


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