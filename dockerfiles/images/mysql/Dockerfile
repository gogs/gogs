#FROM stackbrew/ubuntu:13.10
FROM stackbrew/ubuntu
MAINTAINER  Meaglith Ma <genedna@gmail.com> (@genedna)

RUN apt-get install -y --force-yes software-properties-common

RUN echo "deb http://mirrors.aliyun.com/ubuntu/ saucy main restricted" > /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates main restricted" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-backports main restricted universe multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security main restricted" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security multiverse" >> /etc/apt/sources.list

#RUN add-apt-repository -y "deb http://archive.ubuntu.com/ubuntu $(lsb_release -sc) universe"

RUN apt-get --yes --force-yes update
RUN apt-get --yes --force-yes upgrade 


ENV MYSQL_PASSWORD kuajie8402

RUN echo "mysql-server mysql-server/root_password password $MYSQL_PASSWORD" | debconf-set-selections  
RUN echo "mysql-server mysql-server/root_password_again password $MYSQL_PASSWORD" | debconf-set-selections

RUN apt-get update && apt-get install -y --force-yes mysql-server 
RUN sed -i -e"s/^bind-address\s*=\s*127.0.0.1/bind-address = 0.0.0.0/" /etc/mysql/my.cnf

RUN apt-get autoremove -y
RUN apt-get clean all

EXPOSE 3306

CMD ["/usr/bin/mysqld_safe", "--skip-syslog", "--log-error=/dev/null"]  
