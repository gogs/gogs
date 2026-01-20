#!/bin/sh
set -ex

# Create data directories at runtime (needed when /data is a mounted volume)
mkdir -p /data/gogs /data/git

# Execute the main command
exec "$@"
