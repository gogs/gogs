# Configs
MYSQL_PASSWORD="kuajie8402"
MYSQL_RUN_NAME="gogs_mysql"
typeset -u MYSQL_ALIAS
MYSQL_ALIAS="db"
HOST_PORT="3000"

## Run MySQL image with name
docker run -d --name $MYSQL_RUN_NAME gogs/mysql
#
## Run gogits image and link it to the MySQL image
docker run --link $MYSQL_RUN_NAME:$MYSQL_ALIAS -p $HOST_PORT:3000 gogs/gogits

