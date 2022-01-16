#!/bin/sh
# Crontabs are located by default in /var/spool/cron/crontabs/
# The default configuration is also calling all the scripts in /etc/periodic/${period}

if test -f ./setup; then
    # shellcheck disable=SC2039,SC1091,SC3046
    source ./setup
fi

exec gosu root /usr/sbin/crond -fS
