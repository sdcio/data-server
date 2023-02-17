#!/bin/bash

tag=$1

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

docker pull ghcr.io/nokia/srlinux:$1

id=$(docker create ghcr.io/nokia/srlinux)
mkdir -p yang/srl-$1

docker cp $id:/opt/srlinux/models/. $SCRIPTPATH/yang/srl-$1
docker rm $id
#docker image prune -f

sed -i 's|modifier "invert-match";|//modifier "invert-match";|g' $SCRIPTPATH/yang/srl-$1/srl_nokia/models/common/srl_nokia-common.yang