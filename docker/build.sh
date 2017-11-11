#!/bin/sh
set -x
set -e

# Set temp environment vars
export GOPATH=/tmp/go
export PATH=/usr/local/go/bin:${PATH}:${GOPATH}/bin
export GO15VENDOREXPERIMENT=1

# Install build deps
apk --no-cache --no-progress add --virtual build-deps build-base linux-pam-dev

# Build Gogs
mkdir -p ${GOPATH}/src/github.com/gogits/
ln -s /app/gogs/build ${GOPATH}/src/github.com/gogits/gogs
cd ${GOPATH}/src/github.com/gogits/gogs
# Needed since git 2.9.3 or 2.9.4
git config --global http.https://gopkg.in.followRedirects true
make build TAGS="sqlite cert pam"

# Cleanup GOPATH
rm -r $GOPATH

# Remove build deps
apk --no-progress del build-deps

# Create git user for Gogs
addgroup -S git
adduser -G git -H -D -g 'Gogs Git User' git -h /data/git -s /bin/bash && usermod -p '*' git && passwd -u git
echo "export GOGS_CUSTOM=${GOGS_CUSTOM}" >> /etc/profile
