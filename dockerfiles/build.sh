# Configs of the docker images, you might have specify your own configs here.
MYSQL_PASSWORD="YOUR_MYSQL_PASSWORD"
MYSQL_RUN_NAME="YOUR_MYSQL_RUN_NAME"
HOST_PORT="YOUR_HOST_PORT"

# Replace the mysql root password in MySQL image Dockerfile.
sed -i "s/THE_MYSQL_PASSWORD/$MYSQL_PASSWORD/g" images/mysql/Dockerfile
# Replace the mysql root password in gogits image Dockerfile.
sed -i "s/THE_MYSQL_PASSWORD/$MYSQL_PASSWORD/g" images/gogits/deploy.sh

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
echo "Now we have the MySQL image(running) and gogs image, use the follow command to start gogs service:'
echo -e "\033[33m docker run -i -t --link $MYSQL_RUN_NAME:db -p $HOST_PORT:3000 gogs/gogits \033[0m"

