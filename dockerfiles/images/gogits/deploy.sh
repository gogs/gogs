# deploy.sh in gogits image, replace the configs and run gogs

## Replace the mysql password
MYSQL_PASSWORD=THE_MYSQL_PASSWORD
MYSQL_ALIAS=DB
MYSQL_PASSWORD_LINE=`awk '$0 ~ str{print NR+1}' str="USER = root" $GOPATH/src/github.com/gogits/gogs/conf/app.ini`

sed -i "${MYSQL_PASSWORD_LINE}s/.*$/PASSWD = $MYSQL_PASSWORD/g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini 

## Replace the mysql address and port
# When using --link in docker run, the mysql image's info looks like this:
# DB_PORT=tcp://172.17.0.2:3306
# DB_PORT_3306_TCP_PORT=3306
# DB_PORT_3306_TCP_PROTO=tcp
# DB_PORT_3306_TCP_ADDR=172.17.0.2
sed -i "/HOST = 127.0.0.1:3306/c\HOST = $DB_PORT_3306_TCP_ADDR:$DB_PORT_3306_TCP_PORT" $GOPATH/src/github.com/gogits/gogs/conf/app.ini
cd $GOPATH/src/github.com/gogits/gogs/ 

# The sudo is a must here, or the go within docker container won't get the current user by os.Getenv("USERNAME")
sudo ./gogs web
