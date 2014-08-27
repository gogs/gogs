#!/bin/sh

db_root_passwd=rootpass
db_username=gogs
db_password=password
db_database_name=gogs

DOCKER_BIN=$(which docker.io || which docker)
if [ -z "$DOCKER_BIN" ] ; then
    echo "Please install docker. You can install docker by running \"wget -qO- https://get.docker.io/ | sh\"."
    exit 1
fi

$DOCKER_BIN build -t gogs ./docker &&\
$DOCKER_BIN run --name mysql_gogs -d -e MYSQL_ROOT_PASSWORD=$db_root_passwd -e MYSQL_DATABASE=$db_database_name -e MYSQL_USER=$db_username -e MYSQL_PASSWORD=$db_password mysql:latest &&\
$DOCKER_BIN run --name gogs -d -p 3000:3000 --link mysql_gogs:db gogs

echo "GoGS is up. Visit http://localhost:3000"