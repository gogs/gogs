# deploy.sh in gogits image, replace the configs and run gogs

## Replace the database password
DB_TYPE=THE_DB_TYPE
DB_PASSWORD=THE_DB_PASSWORD
DB_ALIAS=DB
MEM_TYPE=THE_MEM_TYPE

DB_TYPE_LINE=`awk '$0 ~ str{print NR}' str="DB_TYPE = mysql" $GOPATH/src/github.com/gogits/gogs/conf/app.ini`
DB_PASSWORD_LINE=`awk '$0 ~ str{print NR+1}' str="USER = root" $GOPATH/src/github.com/gogits/gogs/conf/app.ini`

sed -i "${DB_TYPE_LINE}s/.*$/DB_TYPE = $DB_TYPE/g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini 
sed -i "${DB_PASSWORD_LINE}s/.*$/PASSWD = $DB_PASSWORD/g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini 



if [ $MEM_TYPE != "" ]
  then
  MEM_HOST_LINE=`awk '$0 ~ str{print NR+6}' str="ADAPTER = memory" $GOPATH/src/github.com/gogits/gogs/conf/app.ini`
                
  _MEM_ADDR=`echo $MEM_PORT | cut -d '/' -f 3 | cut -d ':' -f 1`
  _MEM_PORT=`echo $MEM_PORT | cut -d '/' -f 3 | cut -d ':' -f 2`

  # take advantage of memory db for adapter and provider
  sed -i "s/ADAPTER = memory/ADAPTER = $MEM_TYPE/g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini
  # Comment the memory interval since we don't use 'memory' as adapter
  sed -i "s/INTERVAL = 60/;INTERVAL = 60/g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini


  case $MEM_TYPE in
    "redis")
    # Modify the adapter host
    sed -i "${MEM_HOST_LINE}s/.*/HOST = $_MEM_ADDR:$_MEM_PORT/" $GOPATH/src/github.com/gogits/gogs/conf/app.ini
    sed -i "s/PROVIDER = file/PROVIDER = $MEM_TYPE/g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini
    # Modify the provider config
    sed -i "s#PROVIDER_CONFIG = data/sessions#PROVIDER_CONFIG = $_MEM_ADDR:$_MEM_PORT#g" $GOPATH/src/github.com/gogits/gogs/conf/app.ini
    ;;

    "memcache")
      # Modify the adapter host
      sed -i "${MEM_HOST_LINE}s/.*/HOST = $_MEM_ADDR:$_MEM_PORT/" $GOPATH/src/github.com/gogits/gogs/conf/app.ini
    ;;
  esac

fi


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
