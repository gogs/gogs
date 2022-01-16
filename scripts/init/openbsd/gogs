#!/bin/sh
#
# $OpenBSD$
# shellcheck disable=SC2034,SC1091,SC2154,SC2086

daemon="/home/git/gogs/gogs"
daemon_user="git"
daemon_flags="web"

gogs_directory="/home/git/gogs"

rc_bg=YES

. /etc/rc.d/rc.subr

rc_start() {
	${rcexec} "cd ${gogs_directory}; ${daemon} ${daemon_flags} ${_bg}"
}

rc_cmd $1
