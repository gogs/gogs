# Configs of the docker images, you might have specify your own configs here.

DB_TYPE="YOUR_DB_TYPE"            # type of database, support 'mysql' and 'postgres'
MEM_TYPE="YOUR_MEM_TYPE"          # type of memory database, support 'redis' and 'memcache'
DB_PASSWORD="YOUR_DB_PASSWORD"    # The database password.
DB_RUN_NAME="YOUR_DB_RUN_NAME"    # The --name option value when run the database image.
MEM_RUN_NAME="YOUR_MEM_RUN_NAME"  # The --name option value when run the mem database image.
HOST_PORT="YOUR_HOST_PORT"        # The port on host, which will be redirected to the port 3000 inside gogs container.

# apt source, you can select 'nchc'(mirror in Taiwan) or 'aliyun'(best for mainlance China users) according to your network, if you could connect to the official unbunt mirror in a fast speed, just leave it to "".
APT_SOURCE=""

DOCKER_BIN=$(which docker.io || which docker)
if [ -z "$DOCKER_BIN" ] ; then
    echo "Please install docker. You can install docker by running \"wget -qO- https://get.docker.io/ | sh\"."
    exit 1
fi

# Replace the database root password in database image Dockerfile.
sed -i "s/THE_DB_PASSWORD/$DB_PASSWORD/g" images/$DB_TYPE/Dockerfile
# Replace the database root password in gogits image deploy.sh file. 
sed -i "s/THE_DB_PASSWORD/$DB_PASSWORD/g" images/gogits/deploy.sh
# Replace the apt source in gogits image Dockerfile. 
sed -i "s/#$APT_SOURCE#//" images/gogits/Dockerfile
# Uncomment the installation of database lib in gogs Dockerfile
sed -i "s/#$DB_TYPE#//" images/gogits/Dockerfile
# Replace the database type in gogits image deploy.sh file. 
sed -i "s/THE_DB_TYPE/$DB_TYPE/g" images/gogits/deploy.sh

if [ $MEM_TYPE != "" ]
  then
  # Replace the mem configs in deploy.sh
  sed -i "s/THE_MEM_TYPE/$MEM_TYPE/g" images/gogits/deploy.sh
  # Uncomment the installation of go mem lib
  sed -i "s/#$MEM_TYPE#//" images/gogits/Dockerfile

  # Add the tags when get gogs
  sed -i "s#RUN go get -u -d github.com/gogits/gogs#RUN go get -u -d -tags $MEM_TYPE github.com/gogits/gogs#g" images/gogits/Dockerfile
  # Append the tag in gogs build
  GOGS_BUILD_LINE=`awk '$0 ~ str{print NR}' str="go build" images/gogits/Dockerfile`
  # Append the build tags
  sed -i "${GOGS_BUILD_LINE}s/$/ -tags $MEM_TYPE/" images/gogits/Dockerfile

  cd images/$MEM_TYPE
  $DOCKER_BIN build -t gogits/$MEM_TYPE .
  $DOCKER_BIN run -d --name $MEM_RUN_NAME gogits/$MEM_TYPE
  MEM_LINK=" --link $MEM_RUN_NAME:mem "
  cd ../../
fi

# Build the database image
cd images/$DB_TYPE
$DOCKER_BIN build -t gogits/$DB_TYPE .
#


## Build the gogits image
cd ../gogits

$DOCKER_BIN build -t gogits/gogs .

#sed -i "s#RUN go get -u -tags $MEM_TYPE github.com/gogits/gogs#RUN go get -u github.com/gogits/gogs#g" Dockerfile

# Remove the appended tags in go build line(if there is any)
sed -i "s/ -tags $MEM_TYPE//" Dockerfile

#
## Run MySQL image with name
$DOCKER_BIN run -d --name $DB_RUN_NAME gogits/$DB_TYPE
#
## Run gogits image and link it to the database image
echo "Now we have the $DB_TYPE image(running) and gogs image, use the follow command to start gogs service:"
echo -e "\033[33m $DOCKER_BIN run -i -t --link $DB_RUN_NAME:db $MEM_LINK -p $HOST_PORT:3000 gogits/gogs \033[0m"

