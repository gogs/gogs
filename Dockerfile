FROM alpine:3.5

# Install system utils & Gogs runtime dependencies
ADD https://github.com/tianon/gosu/releases/download/1.9/gosu-amd64 /usr/sbin/gosu
RUN chmod +x /usr/sbin/gosu \
  && echo http://dl-2.alpinelinux.org/alpine/edge/community/ >> /etc/apk/repositories \
  && apk --no-cache --no-progress add \
    bash \
    ca-certificates \
    curl \
    git \
    linux-pam \
    openssh \
    s6 \
    shadow \
    socat \
    tzdata

ENV GOGS_CUSTOM /data/gogs

# Configure LibC Name Service
COPY docker/nsswitch.conf /etc/nsswitch.conf
COPY docker /app/gogs/docker
COPY templates /app/gogs/templates
COPY public /app/gogs/public

WORKDIR /app/gogs/build
COPY . .

RUN    ./docker/build-go.sh \
    && ./docker/build.sh \
    && ./docker/finalize.sh

# Configure Docker Container
VOLUME ["/data"]
EXPOSE 22 3000
ENTRYPOINT ["/app/gogs/docker/start.sh"]
CMD ["/bin/s6-svscan", "/app/gogs/docker/s6/"]
