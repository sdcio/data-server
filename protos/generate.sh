#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

SCHEMA_OUT_DIR=$SCRIPTPATH/schema_server

mkdir -p $SCHEMA_OUT_DIR
protoc --go_out=$SCHEMA_OUT_DIR --go-grpc_out=$SCHEMA_OUT_DIR -I $SCRIPTPATH $SCRIPTPATH/schema.proto $SCRIPTPATH/data.proto

