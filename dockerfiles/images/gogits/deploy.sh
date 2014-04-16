# deploy.sh in gogits image, replace the configs and run gogs

## Replace the database password
DB_TYPE=THE_DB_TYPE
DB_PASSWORD=THE_DB_PASSWORD
DB_ALIAS=DB
DB_TYPE_LINE=`awk '$0 ~ str{print NR}' str="DB_TYPE = mysql" $GOPATH/src/github.com/gogits/gogs/conf/app.ini`
DB_PASSWORD_LINE=`awk '$0 ~ str{print NR+1}' str="USER = root" $GOPATH/src/github.com/gogits/gogs/conf/app.ini`

sed -i "${DB_TYPE_LINE}s/.*$/DB_TYPE = $DB_TYPE/g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini 
sed -i "${DB_PASSWORD_LINE}s/.*$/PASSWD = $DB_PASSWORD/g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini 

## Replace the database address and port
# When using --link in docker run, the database image's info looks like this:
# DB_PORT=tcp://172.17.0.2:3306
# DB_PORT_3306_TCP_PORT=3306
# DB_PORT_3306_TCP_PROTO=tcp
# DB_PORT_3306_TCP_ADDR=172.17.0.2
#sed -i "/HOST = 127.0.0.1:3306/c\HOST = $DB_PORT_3306_TCP_ADDR:$DB_PORT_3306_TCP_PORT" $GOPATH/src/github.com/gogits/gogs/conf/app.ini
sed -i "/HOST = 127.0.0.1:3306/c\HOST = `echo $DB_PORT | cut -d '/' -f 3`" $GOPATH/src/github.com/gogits/gogs/conf/app.ini

cd $GOPATH/src/github.com/gogits/gogs/ 

# The sudo is a must here, or the go within docker container won't get the current user by os.Getenv("USERNAME")
sudo ./gogs web
