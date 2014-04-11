#FROM stackbrew/ubuntu:13.10
#FROM stackbrew/ubuntu
FROM   stackbrew/ubuntu:saucy
MAINTAINER  Meaglith Ma <genedna@gmail.com> (@genedna)

ENV DEBIAN_FRONTEND noninteractive

RUN apt-get install -y --force-yes software-properties-common
RUN add-apt-repository -y "deb http://archive.ubuntu.com/ubuntu $(lsb_release -sc) universe"
RUN apt-get --yes --force-yes update
RUN apt-get --yes --force-yes upgrade 

ENV MYSQL_PASSWORD THE_MYSQL_PASSWORD

RUN echo "mysql-server mysql-server/root_password password $MYSQL_PASSWORD" | debconf-set-selections  
RUN echo "mysql-server mysql-server/root_password_again password $MYSQL_PASSWORD" | debconf-set-selections

RUN apt-get update && apt-get install -y --force-yes mysql-server 
RUN sed -i -e"s/^bind-address\s*=\s*127.0.0.1/bind-address = 0.0.0.0/" /etc/mysql/my.cnf

RUN service mysql restart

RUN echo "mysql -uroot -p$MYSQL_PASSWORD -e 'drop database if exists gogs;'" >> import.sh  
RUN echo "mysql -uroot -p$MYSQL_PASSWORD -e 'create database gogs;'" >> import.sh  
RUN chmod +x import.sh

RUN apt-get autoremove -y
RUN apt-get clean all

RUN    /usr/sbin/mysqld & \
    sleep 10s &&\
    echo "GRANT ALL PRIVILEGES ON *.* TO 'root'@'%' IDENTIFIED BY '$MYSQL_PASSWORD' WITH GRANT OPTION; FLUSH PRIVILEGES" | mysql -p$MYSQL_PASSWORD &&\
    ./import.sh

EXPOSE 3306

CMD ["/usr/bin/mysqld_safe", "--skip-syslog", "--log-error=/dev/null"]  
