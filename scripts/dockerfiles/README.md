### Install Gogs With Docker

Deploying gogs in [Docker](http://www.docker.io/) is just as easy as eating a pie, what you do is just open the `dockerfiles/build.sh` file, replace the configs:

```
DB_TYPE="YOUR_DB_TYPE"            # type of database, support 'mysql' and 'postgres'
MEM_TYPE="YOUR_MEM_TYPE"          # type of memory database, support 'redis' and 'memcache'
DB_PASSWORD="YOUR_DB_PASSWORD"    # The database password.
DB_RUN_NAME="YOUR_DB_RUN_NAME"    # The --name option value when run the database image.
MEM_RUN_NAME="YOUR_MEM_RUN_NAME"  # The --name option value when run the mem database image.
HOST_PORT="YOUR_HOST_PORT"        # The port on host, which will be redirected to the port 3000 inside gogs container.
```

And run:
```
cd dockerfiles
./build.sh
```

The build might take some time, just be paient. After it finishes, you will receive the message:

```
Now we have the MySQL image(running) and gogs image, use the follow command to start gogs service( the content might be different, according to your own configs):
 docker run -i -t --link YOUR_DB_RUN_NAME:db  --link YOUR_MEM_RUN_NAME:mem  -p YOUR_HOST_PORT:3000 gogits/gogs 
```

Just follow the message, run:

```
 docker run -i -t --link YOUR_DB_RUN_NAME:db  --link YOUR_MEM_RUN_NAME:mem  -p YOUR_HOST_PORT:3000 gogits/gogs 
```

Now we have gogs running! Open the browser and navigate to:

```
http://YOUR_HOST_IP:YOUR_HOST_PORT
```

Let's 'gogs'!
Ouya~
