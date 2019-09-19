#!/bin/sh

### Custom user script for gogs
### First param is:
###  "start" (call at start entware),
###  "stop" (call before stop entware),
###
### Note the additional requirements for gogs on ddwrt: shadow user, group, sudo, daemonize

# pid so we know we're running
PIDFILE="/opt/var/run/gogs.pid"
# will need shadow users and groups, sorry - this adds complexity.
USER="gogs"
# go paths
GOROOT="/opt/bin/go"
GOPATH="/opt/go"
# gogs binary location
GOGSBIN="$GOPATH/src/github.com/gogs/gogs/gogs"
# in case you need to see logs for this daemonized gog
DAEMONIZE_LOG="/tmp/gogs.daemon.log"
# SQL can start up slower than normal on DDWRT, use this string to validate if it's up (/opt/bin/netstat -ln |grep "THE STRING YOU SET FOR BELOW VARIABLE"
SQL_STRING=" 127.0.0.1:3306 "

# items for start
PROC="gogs"
DESC=$PROC
PREARGS="/opt/bin/daemonize -v -o $DAEMONIZE_LOG -u $USER -c $GOPATH -p $PIDFILE -E GOROOT=\"$GOROOT\" -E GOPATH=\"$GOPATH\""
ARGS="web"

# legacy RC stuff
ENABLED=yes

# from rc.func
ansi_red="\033[1;31m";ansi_white="\033[1;37m";ansi_green="\033[1;32m";ansi_yellow="\033[1;33m";ansi_blue="\033[1;34m";
ansi_bell="\007";ansi_blink="\033[5m";ansi_std="\033[m";ansi_rev="\033[7m";ansi_ul="\033[4m";

start() {
        # check if we have our user.
        grep -q "^$USER" /etc/passwd || {
             echo -e -n "$ansi_red User $user doesn't exist in /etc/passwd. Exiting.\n$ansi_std"
             exit 1
        }
        if [ -f "$DAEMONIZE_LOG" ]; then rm $DAEMONIZE_LOG; fi
        echo -e -n "$ansi_white Starting $DESC... $ansi_std"
        export GOROOT=$GOROOT
        export GOPATH=$GOPATH
        export PATH=/bin:/usr/bin:/sbin:/usr/sbin:/jffs/sbin:/jffs/bin:/jffs/usr/sbin:/jffs/usr/bin:/mmc/sbin:/mmc/bin:/mmc/usr/sbin:/mmc/usr/bin:/opt/sbin:/opt/bin:/opt/usr/sbin:/ opt/usr/bin:$GOROOT/bin:$GOPATH
        # Wait for SQL
        for n in `seq 10`;
        do
          /opt/bin/netstat -ln |grep -v proc |grep -q "$SQL_STRING" && $PREARGS $GOGSBIN $ARGS > /dev/null && break
          sleep 1
        done

        COUNTER=0
        LIMIT=10
        while [ -z "`pidof $PROC`" -a "$COUNTER" -le "$LIMIT" ]; do
                sleep 1;
                COUNTER=`expr $COUNTER + 1`
        done

        if [ -z "`pidof $PROC`" ]
        then
                echo -e "            $ansi_red failed. $ansi_std"
                logger "Failed to start $DESC from $CALLER."
                return 255
        else
                echo -e "            $ansi_green done. $ansi_std"
                logger "Started $DESC from $CALLER."
                return 0
        fi
}

stop() {
        echo -e -n "$ansi_white Shutting down $PROC...\n $ansi_std"
        killall $PROC 2>/dev/null
        if [ -f "$PIDFILE" ]
        then
                rm "$PIDFILE"
        fi
        COUNTER=0
        LIMIT=10
        while [ -n "`pidof $PROC`" -a "$COUNTER" -le "$LIMIT" ]; do
                sleep 1;
                COUNTER=`expr $COUNTER + 1`
        done
}

status() {
    echo -e -n "$ansi_white Checking $DESC... \n"
    if [ -n "`pidof $PROC`" ]
    then
        echo -e "            $ansi_green alive. $ansi_std";
        return 0
    else
        echo -e "            $ansi_red dead. $ansi_std";
        return 1
    fi
}

die() {
     echo -e -n "$ansi_white Killing $PROC... $ansi_std"
     killall -9 $PROC 2>/dev/null
}

case "$1" in
start)
  start
  ;;
stop)
  stop
  ;;
kill)
  die
  ;;
status | check)
  status
  ;;
restart)
  stop
  start
  ;;
*)
   echo "Usage: $0 {start|stop|kill|restart}"
   exit 1
   ;;
esac
