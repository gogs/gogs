#!/usr/bin/env bash
# Stops the process if something fails
set -xe
echo $GOPATH
ls $GOPATH
go get
# create the application binary that eb uses
GOOS=linux GOARCH=amd64 go build -o bin/application application.go
