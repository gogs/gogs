#!/usr/bin/env bash

docker-compose stop
docker container prune
docker volume rm local_postgres_data
docker volume rm local_postgres_data_backup
