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
CLIENT=$SCRIPTPATH/../bin/client

$CLIENT -a clab-distributed-data-server:56000 datastore get --ds srl1
$CLIENT -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default
$CLIENT -a clab-distributed-data-server:56000 datastore get --ds srl1

$CLIENT -a clab-distributed-data-server:56000 datastore get --ds srl2
$CLIENT -a clab-distributed-data-server:56000 datastore create --ds srl2 --candidate default
$CLIENT -a clab-distributed-data-server:56000 datastore get --ds srl2

echo "start"
date -Ins
for i in $(seq 1 1000);
do 
# date -Ins
$CLIENT -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  --update interface[name=ethernet-1/1]/admin-state:::enable \
                                                        --update interface[name=ethernet-1/1]/vlan-tagging:::true \
                                                        --update interface[name=ethernet-1/1]/description:::interface_desc$i \
                                                        --update interface[name=ethernet-1/1]/subinterface[index=$i]/admin-state:::enable \
                                                        --update interface[name=ethernet-1/1]/subinterface[index=$i]/type:::bridged \
                                                        --update interface[name=ethernet-1/1]/subinterface[index=$i]/description:::subinterface_desc$i \
                                                        --update interface[name=ethernet-1/1]/subinterface[index=$i]/vlan/encap/single-tagged/vlan-id:::$((i+1)) > /dev/null
#
# date -Ins
done
echo "sets"
date -Ins
# $CLIENT data diff --ds srl1 --candidate default > /dev/null
# date
$CLIENT -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
echo "commit"
date -Ins
