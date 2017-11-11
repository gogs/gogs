#!/bin/sh
# Finalize the build

set -x
set -e

# Move to final place
mv /app/gogs/build/gogs /app/gogs/

# Final cleaning
rm -rf /app/gogs/build
rm /app/gogs/docker/build.sh
rm /app/gogs/docker/build-go.sh
rm /app/gogs/docker/finalize.sh
rm /app/gogs/docker/nsswitch.conf
rm /app/gogs/docker/README.md

rm -rf /tmp/go
rm -rf /usr/local/go
