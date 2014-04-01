FROM stackbrew/ubuntu:13.10
MAINTAINER  Meaglith Ma <genedna@gmail.com> (@genedna)

RUN echo "deb http://mirrors.aliyun.com/ubuntu/ saucy main restricted" > /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates main restricted" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-backports main restricted universe multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security main restricted" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security multiverse" >> /etc/apt/sources.list

#ENV MYSQL_PASSWORD docker

#RUN echo "mysql-server mysql-server/root_password password $MYSQL_PASSWORD" | debconf-set-selections  
#RUN echo "mysql-server mysql-server/root_password_again password $MYSQL_PASSWORD" | debconf-set-selections

RUN apt-get update && apt-get install -y --force-yes mysql-server 
RUN sed -i -e"s/^bind-address\s*=\s*127.0.0.1/bind-address = 0.0.0.0/" /etc/mysql/my.cnf

RUN apt-get autoremove -y
RUN apt-get clean all

EXPOSE 3306
CMD ["/usr/bin/mysqld_safe", "--skip-syslog", "--log-error=/dev/null"]  
