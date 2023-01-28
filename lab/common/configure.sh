#!/bin/bash

labname=$1

# build comma separated srl nodes names
nodes=$(docker ps -f label=clab-node-kind=srl -f label=containerlab=${labname} --format {{.Names}} | paste -s -d, -)

gnmic_args="-u admin -p NokiaSrl1! -a ${nodes} --skip-verify"

gnmic $gnmic_args set --request-file ./interfaces/template.gotmpl --request-vars ./interfaces/vars.yaml
