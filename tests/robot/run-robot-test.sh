#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

cd $SCRIPTPATH

git clone https://github.com/nokia/srlinux-yang-models.git
cd srlinux-yang-models
./get-all-modules.sh

cd $SCRIPTPATH
robot --consolecolors on ./tests/collocated/colloc.robot