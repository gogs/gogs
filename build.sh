#!/usr/bin/env bash
# Stops the process if something fails
set -xe
go get
# create the application binary that eb uses
GOOS=linux GOARCH=amd64 go build -o bin/application application.go
