# Configs
MYSQL_PASSWORD="kuajie8402"
MYSQL_RUN_NAME="gogs_mysql"
typeset -u MYSQL_ALIAS
MYSQL_ALIAS="db"
HOST_PORT="3000"

# Replace the mysql root password in MySQL image Dockerfile.
sed -i "s/THE_MYSQL_PASSWORD/$MYSQL_PASSWORD/g" images/mysql/Dockerfile
# Replace the mysql root password in gogits image Dockerfile.
sed -i "s/THE_MYSQL_PASSWORD/$MYSQL_PASSWORD/g" images/gogits/deploy.sh
sed -i "s/THE_MYSQL_ALIAS/$MYSQL_ALIAS/g" images/gogits/deploy.sh


# Build the MySQL image
cd images/mysql
docker build -t gogs/mysql .
#
## Build the gogits image
cd images/gogits
docker build -t gogs/gogits .
#
## Run MySQL image with name
docker run -d --name $MYSQL_RUN_NAME gogs/mysql
#
## Run gogits image and link it to the MySQL image
docker run --link $MYSQL_RUN_NAME:$MYSQL_ALIAS -p $HOST_PORT:3000 gogs/gogits

