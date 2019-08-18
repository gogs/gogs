#!/bin/sh
# Finalize the build

set -x
set -e

# Create git user for Gogs
addgroup -S git
adduser -G git -H -D -g 'Gogs Git User' git -h /data/git -s /bin/bash && usermod -p '*' git && passwd -u git
echo "export GOGS_CUSTOM=${GOGS_CUSTOM}" >> /etc/profile

# Final cleaning
rm -rf /app/gogs/build
rm /app/gogs/docker/build.sh
rm /app/gogs/docker/build-go.sh
rm /app/gogs/docker/finalize.sh
rm /app/gogs/docker/nsswitch.conf
rm /app/gogs/docker/README.md
