#!/bin/sh

PID="log/supervisord.pid"
CONF="etc/supervisord.conf"

EXEPATH='/usr/bin/gogs_start'
if [ ! -f $EXEPATH ]; then
    gogs_scripts_path=$(cd `dirname $0`; pwd)
    echo $gogs_scripts_path
    sudo ln -s $gogs_scripts_path'/start.sh' /usr/bin/gogs_start
fi

LOGDIR="log"
if [ ! -d $LOGDIR ]; then
    mkdir $LOGDIR
fi

stop() {
    if [ -f $PID ]; then
        kill `cat -- $PID`
        rm -f -- $PID
        echo "stopped"
    fi
}

start() {
    echo "starting"
    if [ ! -f $PID ]; then
        supervisord -c $CONF
        echo "started"
    fi
}

case "$1" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    restart)
        stop
        start
        ;;
    *)
        echo "Usage: $0 {start|stop|restart}"
esac