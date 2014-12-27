Docker
======

TOOLS ARE WRITTEN FOR TESTING AND TO SEE WHAT IT IS!

For this to work you will need the nifty docker tool [fig].

The most simple setup will look like this:

```sh
./assemble_blocks.sh docker_gogs w_db option_db_mysql
fig up

```

That's it. You have GoGS running in docker linked to a MySQL docker container.

Now visit http://localhost:3000/ and give details for the admin account an you're up and running.


How does it work
----------------

`./assemble_blocks.sh` will look in `blocks` for subdirectories.
In the subdirectories there are three relevant files: `Dockerfile`, `config` and `fig`.

`Dockerfile` will be copied to `docker/` (also means last `Dockerfile` wins).

The `config` file contains lines which will in the gogs docker container end up in `$GOGS_PATH/custom/config/app.ini` and by this gogs will be configured.
Here you can define things like the MySQL server for your database block.

The `fig` file will just be added to `fig.yml`, which is used by fig to manage your containers.
This includes container linking!

Just have a look at them and it will be clear how to write your own blocks.

Just some things

    - all files (`Dockerfile`, `fig` and `config`) are optional
    - the gogs block should always be the first block

Currently the blocks are designed that, the blocks that start with `docker` pull in the base docker image.
Then one block starting with `w` defines, what containers should be linked to the gogs container.
For every option in the `w` block you need to add an `option` container.

Example:

```sh
./assemble_blocks.sh docker_gogs w_db_cache option_db_mysql option_cache_redis
```


More sophisticated Example
--------------------------

Here is a more elaborated example

```sh
./assemble_blocks.sh docker_gogs w_db_cache_session option_db_postgresql option_cache_redis option_session_mysql
fig up
```

This will set up four containters and link them proberly. One for each of

    - gogs
    - database (postgresql)
    - cache (redis)
    - session (mysql)

WARNING: This will not work at the Moment! MySQL session is broken!


Remark
------

After you execute `assemble_blocks.sh` you should always trigger `fig build` to inculde the the new init script `init_gogs.sh` in the docker image.

If you want to use another GoGS docker file, but keep everything else the same, you can create a block, e.g. `docker_gogs_custom`, with only a `Dockerfile` and call

```sh
./assemble_blocks.sh docker_gogs_custom w_db option_database_mysql
```

This will pull in the `Dockerfile` from `docker_gogs` instead of the one from `docker_gogs`.

`Dockerfile`s for the `master` and `dev` branch are provided as `docker_gogs` and `docker_gogs_dev`


[fig]:http://www.fig.sh/
