#Install gogs under ubuntu 14.04 LTS 32bit from source code

##Requirements
### Go Programming Language: Version >= 1.2
### git(bash): Version >= 1.6.6(both server and client) 
### MySQL: Version >= 5.1 or PostgreSQL or NOTHING. 

## Create the user which will run git
sudo  adduser git
su git

## Install git and Mysql-server
sudo apt-get install git
sudo apt-get install mysql-server

## Create database
$mysql -u root -p
mysql> SET GLOBAL storage_engine = 'InnoDB';
mysql> CREATE DATABASE gogs CHARACTER SET utf8 COLLATE utf8_bin;
mysql> GRANT ALL PRIVILEGES ON gogs.* TO 'root'@'localhost' IDENTIFIED BY 'pasword';
mysql> FLUSH PRIVILEGES;
mysql> QUIT

## install go from source
sudo apt-get install build-essential 
sudo apt-get install mercurial
hg clone -r release https://go.googlecode.com/hg/ /home/git/golang/
 

echo export GOROOT=/home/git/golang >>.bashrc
echo export GOARCH=386   >>.bashrc 
echo export GOOS=linux  >>.bashrc 
echo export GOBIN= /home/git/golang/bin  >>.bashrc 
echo export GOPATH=$HOME/app/Go   >>.bashrc 
echo  PATH=${PATH}: /$HOME/golang/bin  >>.bashrc
cd $GOROOT/src
./make.bash

## Download and install dependencies
$ go get -u github.com/gogits/gogs

## Build main program
$ cd $GOPATH/src/github.com/gogits/gogs
$ go build
$ ./start.sh

## At present, you could access gogs from http://localhost:3000

