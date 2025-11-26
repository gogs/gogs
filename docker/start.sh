#!/bin/sh

# Modern, rootless startup script for Gogs
# Supports Kubernetes security contexts: runAsNonRoot, readOnlyRootFilesystem, allowPrivilegeEscalation=false

create_socat_links() {
    # Bind linked docker container to localhost socket using socat
    USED_PORT="3000:22"
    while read -r NAME ADDR PORT; do
        if test -z "$NAME$ADDR$PORT"; then
            continue
        elif echo "$USED_PORT" | grep -E "(^|:)$PORT($|:)" > /dev/null; then
            echo "init:socat  | Can't bind linked container ${NAME} to localhost, port ${PORT} already in use" 1>&2
        else
            SERV_FOLDER=/app/gogs/docker/s6/SOCAT_${NAME}_${PORT}
            mkdir -p "${SERV_FOLDER}"
            CMD="socat -ls TCP4-LISTEN:${PORT},fork,reuseaddr TCP4:${ADDR}:${PORT}"
            # shellcheck disable=SC2039,SC3037
            echo -e "#!/bin/sh\nexec $CMD" > "${SERV_FOLDER}"/run
            chmod +x "${SERV_FOLDER}"/run
            USED_PORT="${USED_PORT}:${PORT}"
            echo "init:socat  | Linked container ${NAME} will be binded to localhost on port ${PORT}" 1>&2
        fi
    done << EOT
    $(env | sed -En 's|(.*)_PORT_([0-9]+)_TCP=tcp://(.*):([0-9]+)|\1 \3 \4|p')
EOT
}

cleanup() {
    # Cleanup SOCAT services and s6 event folder
    # On start and on shutdown in case container has been killed
    rm -rf "$(find /app/gogs/docker/s6/ -name 'event')"
    rm -rf /app/gogs/docker/s6/SOCAT_*
}

create_volume_subfolder() {
    # Create VOLUME subfolders if they don't exist
    # Note: The container now runs as the git user (UID 1000 by default).
    # Ensure volume permissions match the container user.
    for f in /data/gogs/data /data/gogs/conf /data/gogs/log /data/git /data/ssh; do
        if ! test -d $f; then
            if ! mkdir -p $f 2>/dev/null; then
                echo "Warning: Could not create $f - ensure volume has correct permissions" 1>&2
            fi
        fi
    done
}

cleanup
create_volume_subfolder

LINK=$(echo "$SOCAT_LINK" | tr '[:upper:]' '[:lower:]')
if [ "$LINK" = "false" ] || [ "$LINK" = "0" ]; then
    echo "init:socat  | Will not try to create socat links as requested" 1>&2
else
    create_socat_links
fi

CROND=$(echo "$RUN_CROND" | tr '[:upper:]' '[:lower:]')
if [ "$CROND" = "true" ] || [ "$CROND" = "1" ]; then
    echo "init:crond  | Cron Daemon (crond) will be run as requested by s6" 1>&2
    rm -f /app/gogs/docker/s6/crond/down
    /bin/sh /app/gogs/docker/runtime/backup-init.sh
else
    # Tell s6 not to run the crond service
    touch /app/gogs/docker/s6/crond/down 2>/dev/null || true
fi

# Exec CMD or S6 by default if nothing present
if [ $# -gt 0 ];then
    exec "$@"
else
    exec /usr/bin/s6-svscan /app/gogs/docker/s6/
fi
