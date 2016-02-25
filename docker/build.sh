#!/bin/sh
set -x
set -e

# Set temp environment vars
export GOPATH=/tmp/go
export PATH=${PATH}:${GOPATH}/bin

# Install build deps
apk --no-cache --no-progress add --virtual build-deps linux-pam-dev go gcc musl-dev

# Init go environment to build Gogs
mkdir -p ${GOPATH}/src/github.com/gogits/
ln -s /app/gogs/ ${GOPATH}/src/github.com/gogits/gogs
cd ${GOPATH}/src/github.com/gogits/gogs
go get -v -tags "sqlite cert pam"
go build -tags "sqlite cert pam"

# Cleanup GOPATH
rm -r $GOPATH

# Remove build deps
apk --no-progress del build-deps

# Create git user for Gogs
adduser -H -D -g 'Gogs Git User' git -h /data/git -s /bin/bash && passwd -u git
echo "export GOGS_CUSTOM=${GOGS_CUSTOM}" >> /etc/profile
