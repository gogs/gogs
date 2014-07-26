FROM   	    stackbrew/ubuntu:saucy
MAINTAINER  Meaglith Ma <genedna@gmail.com> (@genedna), Lance Ju <juzhenatpku@gmail.com> (@crystaldust)

RUN         apt-get update && apt-get install -y redis-server
# Usually redis doesn't need a password
#RUN         sed -i "s/# requirepass foobared/requirepass THE_REDIS_PASSWORD/g" /etc/redis/redis.conf
EXPOSE      6379
ENTRYPOINT  ["/usr/bin/redis-server"]
CMD ["--bind", "0.0.0.0"]

