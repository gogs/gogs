#!/bin/sh
export GOPATH=/go
export PATH=$GOPATH/bin:/usr/local/go/bin:/$PATH

mkdir -p "$GOPATH/src" "$GOPATH/bin"
chmod -R 777 "$GOPATH"

cd $GOPATH

git config --global http.https://gopkg.in.followRedirects true

git clone --single-branch --branch ${GOGS_VERSION} --depth 1 https://github.com/gogits/gogs ${GOPATH}/src/github.com/gogits/gogs
cd ${GOPATH}/src/github.com/gogits/gogs
make build TAGS="sqlite cert pam"

# Create git user for Gogs
adduser -H -D -g 'Gogs Git User' git -h /data/git -s /bin/bash && passwd -u git
echo "export GOGS_CUSTOM=${GOGS_CUSTOM}" >> /etc/profile