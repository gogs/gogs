#!/bin/sh

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
    # only change ownership if needed, if using an nfs mount this could be expensive
    if [ "$USER:$USER" != "$(stat /data -c '%U:%G')" ]
    then
        # Modify the owner of /data dir, make $USER(git) user have permission to create sub-dir in /data.
        chown -R "$USER:$USER" /data
    fi

    # Create VOLUME subfolder
    for f in /data/gogs/data /data/gogs/conf /data/gogs/log /data/git /data/ssh; do
        if ! test -d $f; then
            gosu "$USER" mkdir -p $f
        fi
    done
}

setids() {
    export USER=git
    PUID=${PUID:-1000}
    PGID=${PGID:-1000}
    groupmod -o -g "$PGID" $USER
    usermod -o -u "$PUID" $USER
}

envsubst_vars() {
    GOGS_DB_TYPE=${GOGS_DB_TYPE:-"sqlite3"} \
    GOGS_DB_HOST=${GOGS_DB_HOST:-"127.0.0.1"} \
    GOGS_DB_PORT=${GOGS_DB_PORT:-"5432"} \
    GOGS_DB_NAME=${GOGS_DB_NAME:-"gogs"} \
    GOGS_SCHEMA=${GOGS_SCHEMA:-"public"} \
    GOGS_DB_USER=${GOGS_DB_USER:-"gogs"} \
    GOGS_DB_PASSWORD=${GOGS_DB_PASSWORD:-"gogs"} \
    GOGS_DB_SSL_MODE=${GOGS_DB_SSL_MODE:-"disable"} \
    GOGS_DEFAULT_BRANCH=${GOGS_DEFAULT_BRANCH:-"main"} \
    GOGS_DOMAIN=${GOGS_DOMAIN:-"localhost"} \
    GOGS_HTTP_PORT=${GOGS_HTTP_PORT:-"3000"} \
    GOGS_EXTERNAL_URL=${GOGS_EXTERNAL_URL:-"http://localhost:3000/"} \
    GOGS_DISABLE_SSH=${GOGS_DISABLE_SSH:-"false"} \
    GOGS_SSH_PORT=${GOGS_SSH_PORT:-"22"} \
    GOGS_START_SSH_SERVER=${GOGS_START_SSH_SERVER:-"false"} \
    GOGS_OFFLINE_MODE=${GOGS_OFFLINE_MODE:-"false"} \
    envsubst < /app/gogs/docker/templates/app.ini > /data/gogs/conf/app.ini
}

setids
cleanup
create_volume_subfolder
envsubst_vars

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
    /bin/sh /app/gogs/docker/runtime/backup-init.sh "${PUID}"
else
    # Tell s6 not to run the crond service
    touch /app/gogs/docker/s6/crond/down
fi

# Exec CMD or S6 by default if nothing present
if [ $# -gt 0 ];then
    exec "$@"
else
    exec /usr/bin/s6-svscan /app/gogs/docker/s6/
fi
