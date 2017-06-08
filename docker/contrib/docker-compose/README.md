# Docker Compose

[Gogs](https://github.com/gogits/gogs) : Go Git Service

Docker Compose v2 (Docker 1.10) with mysql as database

Place docker-compose.yaml in  `~/docker-compose/gogs/docker-compose.yaml` for exemple:

In the `gogs` folder, bring up the stack :

```
# docker-compose up -d
Creating network "gogs_default" with the default driver
Creating volume "gogs_gogs_db_data" with local driver
Creating volume "gogs_gogs_server_data" with local driver
Creating gogs_db
Creating gogs_server
```

Check containers :

```
# docker-compose ps
   Name                  Command               State                       Ports
----------------------------------------------------------------------------------------------------
gogs_db       /entrypoint.sh mysqld            Up      3306/tcp
gogs_server   docker/start.sh /bin/s6-sv ...   Up      0.0.0.0:10022->22/tcp, 0.0.0.0:3000->3000/tcp
```

Access Gogs URL to finalize installation : `http://localhost:3000`

To destroy the stack (`-v` option deletes volumes) :

```
# docker-compose down -v
Stopping gogs_server ... done
Stopping gogs_db ... done
Removing gogs_server ... done
Removing gogs_db ... done
Removing network gogs_default
Removing volume gogs_gogs_db_data
Removing volume gogs_gogs_server_data
```
