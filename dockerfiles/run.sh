# Configs
MYSQL_PASSWORD="kuajie8402"
MYSQL_RUN_NAME="gogs_mysql"
typeset -u MYSQL_ALIAS
MYSQL_ALIAS="db"
HOST_PORT="3000"

DOCKER_BIN=$(which docker.io || which docker)
if [ -z "$DOCKER_BIN" ] ; then
    echo "Please install docker. You can install docker by running \"wget -qO- https://get.docker.io/ | sh\"."
    exit 1
fi

## Run MySQL image with name
$DOCKER_BIN run -d --name $MYSQL_RUN_NAME gogs/mysql
#
## Run gogits image and link it to the MySQL image
$DOCKER_BIN run --link $MYSQL_RUN_NAME:$MYSQL_ALIAS -p $HOST_PORT:3000 gogs/gogits

