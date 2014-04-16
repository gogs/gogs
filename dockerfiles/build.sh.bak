# Configs of the docker images, you might have specify your own configs here.
# type of database, support 'mysql' and 'postgres'
DB_TYPE="postgres"
DB_PASSWORD="YOUR_DB_PASSWORD"
DB_RUN_NAME="YOUR_DB_RUN_NAME"
HOST_PORT="YOUR_HOST_PORT"

# Replace the database root password in database image Dockerfile.
sed -i "s/THE_DB_PASSWORD/$DB_PASSWORD/g" images/$DB_TYPE/Dockerfile
# Replace the database root password in gogits image deploy.sh file. 
sed -i "s/THE_DB_PASSWORD/$DB_PASSWORD/g" images/gogits/deploy.sh
# Replace the database type in gogits image deploy.sh file. 
sed -i "s/THE_DB_TYPE/$DB_TYPE/g" images/gogits/deploy.sh

# Build the database image
cd images/$DB_TYPE
docker build -t gogs/$DB_TYPE .
#
## Build the gogits image
cd ../gogits
docker build -t gogs/gogits .
#
## Run MySQL image with name
docker run -d --name $DB_RUN_NAME gogs/$DB_TYPE
#
## Run gogits image and link it to the database image
echo "Now we have the $DB_TYPE image(running) and gogs image, use the follow command to start gogs service:"
echo -e "\033[33m docker run -i -t --link $DB_RUN_NAME:db -p $HOST_PORT:3000 gogs/gogits \033[0m"

