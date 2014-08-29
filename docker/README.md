Docker
======

TOOLS ARE WRITTEN FOR TESTING AND TO SEE WHAT IT IS!

For this to work you will need the nifty docker tool [fig].

The most simple setup will look like this:

```sh
./assemble_blocks.sh docker_gogs_w_db docker_database_mysql
fig up

```

That's it. You have GoGS running in docker linked to a MySQL docker container.

Now visit http://localhost:3000/ and give details for the admin account an you're up and running.


How does it work
----------------

`./assemble_blocks.sh` will look in `blocks` for subdirectories.
In the subdirectories there are to relevant files: `Dockerfile`, `config` and `fig`.

`Dockerfile` will be copied to `docker/` (also means last `Dockerfile` wins).

The `config` file contains lines which will in the gogs docker container end up in `$GOGS_PATH/custom/config/app.ini` and by this gogs will be configured.
Here you can define things like the MySQL server for your database block.

The `fig` file will just be added to `fig.yml`, which is used by fig to manage your containers.
This inculdes container linking!

Just have a look at them and it will be clear how to write your own blocks.

Just some things

    - all files (`Dockerfile`, `fig` and `config`) are optional
    - the gogs block should always be the first block


More sophisticated Example
--------------------------

Her is a more elaborated example

```sh
./assemble_blocks.sh docker_gogs_w_db_cache_session docker_database_postgresql docker_cache_redis docker_session_mysql
fig up
```

This will set up four containters. One for each of

    - gogs
    - database (postgresql)
    - cache (redis)
    - session (mysql)

WARNING: This will not work at the Moment! MySQL session is broken!


Remark
------

After you change something you should always trigger `fig build` to inculde the the new init script `init_gogs.sh` in the docker image.

If you want to use another GoGS docker file, but keep everything else the same, you can create a block, e.g. `docker_gogs_dev`, with only a `Dockerfile` and call

```sh
./assemble_blocks.sh docker_gogs_w_db docker_gogs_dev docker_database_mysql
```

This will override the `Dockerfile` from `docker_gogs_w_db` with the one from `docker_gogs_dev`


[fig]:http://www.fig.sh/