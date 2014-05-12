#!/bin/sh

echo 'plase remember to modify the command path in etc/conf/supervisord.conf(line 23)'

PID="/tmp/supervisord.pid"
CONF="conf/etc/supervisord.conf"

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