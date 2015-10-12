#!/bin/sh

# Cleanup SOCAT services and s6 event folder
# On start and on shutdown in case container has been killed
rm -rf $(find /app/gogs/docker/s6/ -name 'event')
rm -rf /app/gogs/docker/s6/SOCAT_*

# Create VOLUME subfolder
for f in /data/gogs/data /data/gogs/conf /data/gogs/log /data/git /data/ssh; do
    if ! test -d $f; then
        mkdir -p $f
    fi
done

# Bind linked docker container to localhost socket using socat
env | sed -En 's|(.*)_PORT_([0-9]*)_TCP=tcp://(.*):(.*)|\1_\2 socat -ls TCP4-LISTEN:\2,fork,reuseaddr TCP4:\3:\4|p' | \
while read NAME CMD; do
    mkdir -p /app/gogs/docker/s6/SOCAT_$NAME
    echo -e "#!/bin/sh\nexec $CMD" > /app/gogs/docker/s6/SOCAT_$NAME/run
    chmod +x /app/gogs/docker/s6/SOCAT_$NAME/run
done

# Exec CMD or S6 by default if nothing present
if [ $# -gt 0 ];then
    exec "$@"
else
    exec /usr/bin/s6-svscan /app/gogs/docker/s6/
fi
