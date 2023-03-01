#!/bin/bash

sudo clab dep -c

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

$SCRIPTPATH/../common/configure.sh schema
