#!/bin/sh

# Bind linked docker container to localhost socket using socat
env | sed -En 's|(.*)_PORT_([0-9]*)_TCP=tcp://(.*):(.*)|\1_\2 socat -ls TCP4-LISTEN:\2,fork,reuseaddr TCP4:\3:\4|p' | \
while read NAME CMD; do
    mkdir -p /app/gogs/docker/s6/$NAME
    echo -e "#!/bin/sh\nexec $CMD" > /app/gogs/docker/s6/$NAME/run
    chmod +x /app/gogs/docker/s6/$NAME/run
done

# Exec CMD or S6 by default if nothing present
if [ $# -gt 0 ];then
    exec "$@"
else
    exec /usr/bin/s6-svscan /app/gogs/docker/s6/
fi
