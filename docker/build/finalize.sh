#!/bin/sh

set -xe

# Install gosu
if [ "$(uname -m)" = "aarch64" ]; then
  export arch='arm64'
  export checksum='c216cb762520edb5ab0acb21cb9bc4bcc5f1544d365e8952a21514f04ea233ee'
elif [ "$(uname -m)" = "armv7l" ]; then
  export arch='armhf'
  export checksum='02f58cb01be8cbd981d4850c59ee3249b1039af3b2d43c44e950f90bf18ae0ac'
else
  export arch='amd64'
  export checksum='b1892636c281590f34bc63c1118f058cd9de66c4ea463772e3909672d8b302ac'
fi

wget --quiet https://github.com/tianon/gosu/releases/download/1.20/gosu-${arch} -O /usr/sbin/gosu
echo "${checksum}  /usr/sbin/gosu" | sha256sum -cs
chmod +x /usr/sbin/gosu

# Create git user for Gogs
addgroup -S git
adduser -G git -H -D -g 'Gogs Git User' git -h /data/git -s /bin/bash && usermod -p '*' git && passwd -u git
echo "export GOGS_CUSTOM=${GOGS_CUSTOM}" >> /etc/profile

# Final cleaning
rm -rf /app/gogs/build
rm -rf /app/gogs/docker/build
rm /app/gogs/docker/nsswitch.conf
rm /app/gogs/docker/README.md
