#!/bin/sh
set -x
set -e

# Set temp environment vars
export GOPATH=/tmp/go
export PATH=/usr/local/go/bin:${PATH}:${GOPATH}/bin
export GO15VENDOREXPERIMENT=1

# Install build deps
apt install -y libpam0g-dev

# Build Gogs
mkdir -p ${GOPATH}/src/github.com/gogits/
ln -s /app/gogs/build ${GOPATH}/src/github.com/gogits/gogs
cd ${GOPATH}/src/github.com/gogits/gogs
# Needed since git 2.9.3 or 2.9.4
git config --global http.https://gopkg.in.followRedirects true
make build TAGS="sqlite cert pam"

# Cleanup GOPATH
rm -r $GOPATH

# Create git user for Gogs
addgroup --system git
useradd -o -u 0 -g git -M git -d /data/git -s /bin/bash && \
    usermod -p '*' git && \
    passwd -u git
echo "export GOGS_CUSTOM=${GOGS_CUSTOM}" >> /etc/profile
