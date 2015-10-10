FROM alpine:3.2
MAINTAINER roemer.jp@gmail.com

# Install system utils & Gogs runtime dependencies
ADD https://github.com/tianon/gosu/releases/download/1.6/gosu-amd64 /usr/sbin/gosu
RUN echo "@edge http://dl-4.alpinelinux.org/alpine/edge/main" | tee -a /etc/apk/repositories \
 && echo "@community http://dl-4.alpinelinux.org/alpine/edge/community" | tee -a /etc/apk/repositories \
 && apk -U --no-progress upgrade \
 && apk -U --no-progress add ca-certificates bash git linux-pam s6@edge curl openssh socat \
 && chmod +x /usr/sbin/gosu

ENV GOGS_CUSTOM /data/gogs

COPY . /app/gogs/
WORKDIR /app/gogs/
RUN ./docker/build.sh

# Configure Docker Container
VOLUME ["/data"]
EXPOSE 22 3000
ENTRYPOINT ["docker/start.sh"]
CMD ["/usr/bin/s6-svscan", "/app/gogs/docker/s6/"]
