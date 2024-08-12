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


labname=$1

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

# build comma separated srl nodes names
nodes=$(docker ps -f label=clab-node-kind=srl -f label=containerlab=${labname} --format {{.Names}} | paste -s -d, -)

gnmic_args="-u admin -p NokiaSrl1! -a ${nodes} --skip-verify"

gnmic $gnmic_args set --request-file $SCRIPTPATH/interfaces/template.gotmpl --request-vars $SCRIPTPATH/interfaces/vars.yaml
