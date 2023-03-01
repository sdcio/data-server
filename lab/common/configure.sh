#!/bin/bash

labname=$1

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

# build comma separated srl nodes names
nodes=$(docker ps -f label=clab-node-kind=srl -f label=containerlab=${labname} --format {{.Names}} | paste -s -d, -)

gnmic_args="-u admin -p NokiaSrl1! -a ${nodes} --skip-verify"

gnmic $gnmic_args set --request-file $SCRIPTPATH/interfaces/template.gotmpl --request-vars $SCRIPTPATH/interfaces/vars.yaml
