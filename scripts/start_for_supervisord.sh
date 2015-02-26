#!/bin/sh
IFS='
	'
PATH=/bin:/usr/bin:/usr/local/bin
USER=$(whoami)
HOME=$(grep "^$USER:" /etc/passwd | cut -d: -f6)
export USER HOME PATH

cd "$HOME/gogs" && exec ./gogs web