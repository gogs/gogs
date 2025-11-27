#!/bin/sh

# Modern, rootless startup script for Gogs
# Supports Kubernetes security contexts: runAsNonRoot, allowPrivilegeEscalation=false, capabilities: drop: ALL

create_volume_subfolder() {
    # Create VOLUME subfolders if they don't exist
    # Note: The container runs as the git user (UID 1000 by default).
    # Ensure volume permissions match the container user.
    for f in /data/gogs/data /data/gogs/conf /data/gogs/log /data/git; do
        if ! test -d $f; then
            if ! mkdir -p $f 2>/dev/null; then
                echo "Warning: Could not create $f - ensure volume has correct permissions" 1>&2
            fi
        fi
    done
}

cleanup() {
    # Cleanup s6 event folder on start and shutdown
    rm -rf "$(find /app/gogs/docker/s6/ -name 'event')" 2>/dev/null || true
}

cleanup
create_volume_subfolder

# Exec CMD or S6 by default if nothing present
if [ $# -gt 0 ]; then
    exec "$@"
else
    exec /usr/bin/s6-svscan /app/gogs/docker/s6/
fi
