FROM alpine:3.2
MAINTAINER roemer.jp@gmail.com

# Install system utils & Gogs runtime dependencies
ADD https://github.com/tianon/gosu/releases/download/1.5/gosu-amd64 /usr/sbin/gosu
RUN echo "@edge http://dl-4.alpinelinux.org/alpine/edge/main" | tee -a /etc/apk/repositories \
 && echo "@community http://dl-4.alpinelinux.org/alpine/edge/community" | tee -a /etc/apk/repositories \
 && apk -U --no-progress upgrade \
 && apk -U --no-progress add ca-certificates git linux-pam s6@edge curl openssh socat \
 && chmod +x /usr/sbin/gosu

# Configure SSH
COPY docker/sshd_config /etc/ssh/sshd_config

# Configure Go and build Gogs
ENV GOPATH /tmp/go
ENV PATH $PATH:$GOPATH/bin

COPY . /app/gogs/
WORKDIR /app/gogs/
RUN ./docker/build.sh

ENV GOGS_CUSTOM /data/gogs

# Create git user for Gogs
RUN adduser -D -g 'Gogs Git User' git -h /data/git/ -s /bin/sh && passwd -u git
RUN echo "export GOGS_CUSTOM=/data/gogs" >> /etc/profile

VOLUME ["/data"]
EXPOSE 22 3000
CMD ["./docker/start.sh"]
