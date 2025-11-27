#!/bin/sh

set -xe

# Create git user for Gogs (already created in Dockerfile with fixed UID/GID)
echo "export GOGS_CUSTOM=${GOGS_CUSTOM}" >> /etc/profile

# Create necessary directories with proper permissions
mkdir -p /data/gogs/data /data/gogs/conf /data/gogs/log /data/git /backup
chown -R git:git /data /backup
chmod 755 /data /data/gogs /data/git

# Create home directory symlink for backward compatibility
ln -sfn /data/git /home/git

# Final cleaning
rm -rf /app/gogs/build
rm -rf /app/gogs/docker/build
rm /app/gogs/docker/nsswitch.conf
rm /app/gogs/docker/README.md
