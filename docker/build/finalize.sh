#!/bin/sh

set -xe

# Export GOGS_CUSTOM environment variable for all users
echo "export GOGS_CUSTOM=${GOGS_CUSTOM}" >> /etc/profile

# Create necessary directories with proper permissions
mkdir -p /data/gogs/data /data/gogs/conf /data/gogs/log /data/git /data/ssh /backup
chown -R git:git /data /backup
chmod 755 /data /data/gogs /data/git
chmod 700 /data/ssh

# Create crontabs directory for non-root cron support
mkdir -p /var/spool/cron/crontabs
chown -R git:git /var/spool/cron/crontabs
chmod 700 /var/spool/cron/crontabs

# Create home directory symlink for backward compatibility
ln -sfn /data/git /home/git

# Final cleaning
rm -rf /app/gogs/build
rm -rf /app/gogs/docker/build
rm /app/gogs/docker/nsswitch.conf
rm /app/gogs/docker/README.md
