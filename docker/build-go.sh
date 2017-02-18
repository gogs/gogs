#!/bin/sh
# Build GO version as specified in Dockerfile

# Install build tools
apk add --no-cache --virtual .build-deps bash gcc musl-dev openssl go build-base linux-pam-dev

export GOROOT_BOOTSTRAP="$(go env GOROOT)"

# Download Go
wget -q "$GOLANG_SRC_URL" -O golang.tar.gz
echo "$GOLANG_SRC_SHA256  golang.tar.gz" | sha256sum -c -
tar -C /usr/local -xzf golang.tar.gz
rm golang.tar.gz

# Build
cd /usr/local/go/src
# see https://golang.org/issue/14851
patch -p2 -i /app/gogs/docker/no-pic.patch
./make.bash

# Clean
rm /app/gogs/docker/*.patch
