#!/bin/sh

if test -f ./setup; then
    # shellcheck disable=SC2039,SC1091,SC3046
    source ./setup
fi

exec gosu root /sbin/syslogd -nS -O-
