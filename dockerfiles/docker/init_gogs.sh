#!/bin/sh

mkdir -p $GOGS_CUSTOM_CONF_PATH

#~ Either "dev", "prod" or "test", default is "dev"
echo "RUN_MODE = dev"                                          >> $GOGS_CUSTOM_CONF

echo "[database]"                                               >> $GOGS_CUSTOM_CONF
echo "DB_TYPE = mysql"                                          >> $GOGS_CUSTOM_CONF
echo "HOST = ${DB_PORT_3306_TCP_ADDR}:${DB_PORT_3306_TCP_PORT}" >> $GOGS_CUSTOM_CONF
echo "NAME = ${DB_ENV_MYSQL_DATABASE}"                          >> $GOGS_CUSTOM_CONF
echo "USER = ${DB_ENV_MYSQL_USER}"                              >> $GOGS_CUSTOM_CONF
echo "PASSWD = ${DB_ENV_MYSQL_PASSWORD}"                        >> $GOGS_CUSTOM_CONF

exec "$@"
