# deploy.sh in gogits image
# Script in the gogits image
## Replace the mysql password
MYSQL_PASSWORD=kuajie8402
MYSQL_ALIAS=DB
MYSQL_PASSWORD_LINE=`awk '$0 ~ str{print NR+1}' str="USER = root" $GOPATH/src/github.com/gogits/gogs/conf/app.ini`

sed -e "${MYSQL_PASSWORD_LINE}s/.*$/PASSWD = $MYSQL_PASSWORD/g" conf/app.ini 

## Replace the mysql address and port
# DB_PORT=tcp://172.17.0.2:3306
# DB_PORT_3306_TCP_PORT=3306
# DB_PORT_3306_TCP_PROTO=tcp
sed -e "/HOST = 127.0.0.1:3306/c\HOST = ${MYSQLALIAS}_PORT" app.ini

