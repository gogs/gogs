#!/usr/bin/env bash
# Stops the process if something fails
set -xe

# create the application binary that eb uses
go build -o bin/application application.go
