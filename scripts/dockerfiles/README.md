### Install Gogs With Docker

Deploying gogs using [Docker](http://www.docker.io/) is as easy as pie. Simple
open the `/dockerfiles/build.sh` file and replace the initial configuration
settings:

```
DB_TYPE="YOUR_DB_TYPE"            # type of database, supports either 'mysql' or 'postgres'
MEM_TYPE="YOUR_MEM_TYPE"          # type of memory database, supports either 'redis' or 'memcache'
DB_PASSWORD="YOUR_DB_PASSWORD"    # The database password
DB_RUN_NAME="YOUR_DB_RUN_NAME"    # The --name option value to use when running the database image
MEM_RUN_NAME="YOUR_MEM_RUN_NAME"  # The --name option value to use when running the memory database image
HOST_PORT="YOUR_HOST_PORT"        # The port to expose the app on (redirected to 3000 inside the gogs container)
```

And run:
```
cd dockerfiles
./build.sh
```

The build will take some time, just be patient. After it finishes, it will
display a message that looks like this (the content may be different, depending
on your configuration options):

```
Now we have the MySQL image(running) and gogs image, use the follow command to start gogs service:
docker run -i -t --link YOUR_DB_RUN_NAME:db --link YOUR_MEM_RUN_NAME:mem -p YOUR_HOST_PORT:3000 gogits/gogs
```

To run the container, just copy the above command:

```
docker run -i -t --link YOUR_DB_RUN_NAME:db --link YOUR_MEM_RUN_NAME:mem -p YOUR_HOST_PORT:3000 gogits/gogs
```

Now gogs should be running! Open your browser and navigate to:

```
http://YOUR_HOST_IP:YOUR_HOST_PORT
```

During the installation procedure, use the following information:

- The database type should be whichever `DB_TYPE` you selected above

- The database host should be either `db:5432` or `db:3306` for PostgreSQL and
  MySQL respectively

- The `RUN_USER` should be whichever user you're running the container with.
  Ideally that's `git`, but your individual configuration may vary

- Everything else is configured like a normal gogs installation

Let's 'gogs'!
Ouya~
