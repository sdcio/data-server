#!/bin/bash

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
