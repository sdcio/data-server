#!/bin/bash

tag=$1

docker pull ghcr.io/nokia/srlinux:$1

id=$(docker create ghcr.io/nokia/srlinux)
mkdir -p yang/srl-$1

docker cp $id:/opt/srlinux/models/. yang/srl-$1
docker rm $id
#docker image prune -f

sed -i 's|modifier "invert-match";|//modifier "invert-match";|g' yang/srl-$1/srl_nokia/models/common/srl_nokia-common.yang