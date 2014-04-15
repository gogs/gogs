### Gogs Install With Docker



#### Gogs With MySQL

Deply gogs in [Docker](http://www.docker.io/) is just as easy as eating a pie, what you do is just open the `dockerfiles/build.sh` file, replace the confis:

```
MYSQL_PASSWORD="YOUR_MYSQL_PASSWORD"
MYSQL_RUN_NAME="YOUR_MYSQL_RUN_NAME"
HOST_PORT="YOUR_HOST_PORT"
```

And run:
```
cd dockerfiles
./build.sh
```

The build might take some time, just be paient. After it finishes, you will receive the message:

```
Now we have the MySQL image(running) and gogs image, use the follow command to start gogs service( the content might be different, according to your own configs):
 docker run -i -t --link gogs_mysql:db -p 3333:3000 gogs/gogits
```

Just follow the message, run:

```
 docker run -i -t --link gogs_mysql:db -p 3333:3000 gogs/gogits
```

Now we have gogs running! Open the browser and navigate to:

```
http://YOUR_HOST_IP:YOUR_HOST_PORT
```

Let's 'gogs'!

#### Gogs With PostgreSQL

Installing Gogs with PostgreSQL is nearly the same with installing it with MySQL. What you do is just change the DB_TYPE in build.sh to 'postgres'.

#### Gogs, MySQL With Redis


#### Gogs, MySQL With Memcached


#### Gogs, PostgreSQL With Redis


#### Gogs, PostgreSQL With Memcached




