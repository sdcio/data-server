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


tag=$1

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

docker pull ghcr.io/nokia/srlinux:$1

id=$(docker create ghcr.io/nokia/srlinux:$1)
mkdir -p yang/srl-$1

docker cp $id:/opt/srlinux/models/. $SCRIPTPATH/yang/srl-$1
docker rm $id
#docker image prune -f

sed -i 's|modifier "invert-match";|//modifier "invert-match";|g' $SCRIPTPATH/yang/srl-$1/srl_nokia/models/common/srl_nokia-common.yang
