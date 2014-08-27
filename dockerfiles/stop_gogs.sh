#!/bin/sh


DOCKER_BIN=$(which docker.io || which docker)
if [ -z "$DOCKER_BIN" ] ; then
    echo "Please install docker. You can install docker by running \"wget -qO- https://get.docker.io/ | sh\"."
    exit 1
fi

$DOCKER_BIN stop gogs
$DOCKER_BIN stop mysql_gogs