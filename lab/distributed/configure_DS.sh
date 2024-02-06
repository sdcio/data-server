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


num_vlans=4000
conc=1000

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
CLIENT=$SCRIPTPATH/../../bin/client
BULK=$SCRIPTPATH/../../bin/bulk

echo "srl1/ethernet-1/1 - $num_vlans vlans"
$BULK -a clab-distributed-data-server:56000 --ds srl1 --candidate default1 --vlans $num_vlans --interface ethernet-1/1 --concurrency $conc #--cleanup #&
echo ""

echo "srl2/ethernet-1/1 - $num_vlans vlans"
$BULK -a clab-distributed-data-server:56000 --ds srl2 --candidate default1 --vlans $num_vlans --interface ethernet-1/1 --concurrency $conc #--cleanup #&
echo ""

echo "srl3/ethernet-1/1 - $num_vlans vlans"
$BULK -a clab-distributed-data-server:56000 --ds srl3 --candidate default1 --vlans $num_vlans --interface ethernet-1/1 --concurrency $conc #--cleanup #&
echo ""

echo "srl3/ethernet-1/2 - $num_vlans vlans"
$BULK -a clab-distributed-data-server:56000 --ds srl3 --candidate default2 --vlans $num_vlans --interface ethernet-1/2 --concurrency $conc #--cleanup
echo ""
