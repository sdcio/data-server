#!/bin/bash

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

SCHEMA_OUT_DIR=$SCRIPTPATH/schema_server

mkdir -p $SCHEMA_OUT_DIR
protoc --go_out=$SCHEMA_OUT_DIR/schema_server --go-grpc_out=$SCHEMA_OUT_DIR/schema_server -I $SCRIPTPATH $SCRIPTPATH/schema.proto $SCRIPTPATH/data.proto

