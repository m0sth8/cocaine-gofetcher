#!/bin/bash

CUR_DIR=$(pwd)

echo "Set go path $CUR_DIR"
export GOPATH=$CUR_DIR

echo "Build"
go build gofetcher