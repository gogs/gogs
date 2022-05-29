#!/bin/sh

set -xe

# Install gosu
if [ "$(uname -m)" = "aarch64" ]; then
  export arch='arm64'
  export checksum='73244a858f5514a927a0f2510d533b4b57169b64d2aa3f9d98d92a7a7df80cea'
elif [ "$(uname -m)" = "armv7l" ]; then
  export arch='armhf'
  export checksum='abb1489357358b443789571d52b5410258ddaca525ee7ac3ba0dd91d34484589'
else
  export arch='amd64'
  export checksum='bd8be776e97ec2b911190a82d9ab3fa6c013ae6d3121eea3d0bfd5c82a0eaf8c'
fi

wget --quiet https://github.com/tianon/gosu/releases/download/1.14/gosu-${arch} -O /usr/sbin/gosu
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
