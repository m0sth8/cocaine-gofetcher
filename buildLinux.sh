#!/bin/bash

CUR_DIR=$(pwd)
PLATFORM="linux/amd64"

echo "Set go path $CUR_DIR/src"
export GOPATH=$CUR_DIR

echo "Set os and arg to $PLATFORM"
export GOOS=linux
export GOARCH=amd64

echo "Build"
go build gofetcher