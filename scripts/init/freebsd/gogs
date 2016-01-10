#!/bin/sh
#
# $FreeBSD$
#
# PROVIDE: gogs
# REQUIRE: NETWORKING SYSLOG
# KEYWORD: shutdown
#
# Add the following lines to /etc/rc.conf to enable gogs:
#
#gogs_enable="YES"

. /etc/rc.subr

name="gogs"
rcvar="gogs_enable"

load_rc_config $name

: ${gogs_user:="git"}
: ${gogs_enable:="NO"}
: ${gogs_directory:="/home/git"}

command="${gogs_directory}/gogs web"
procname="$(echo $command |cut -d' ' -f1)"

pidfile="${gogs_directory}/${name}.pid"

start_cmd="${name}_start"
stop_cmd="${name}_stop"

gogs_start() {
	cd ${gogs_directory}
	export USER=${gogs_user}
	export HOME=/usr/home/${gogs_user}
	/usr/sbin/daemon -f -u ${gogs_user} -p ${pidfile} $command
}

gogs_stop() {
	if [ ! -f $pidfile ]; then
		echo "GOGS PID File not found. Maybe GOGS is not running?"
	else
		kill $(cat $pidfile)
	fi
}

run_rc_command "$1"
