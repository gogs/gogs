FROM alpine:3.5

# Components versions
ENV GOLANG_VERSION 1.8
ENV GOLANG_SRC_URL https://golang.org/dl/go$GOLANG_VERSION.src.tar.gz
ENV GOLANG_SRC_SHA256 406865f587b44be7092f206d73fc1de252600b79b3cacc587b74b5ef5c623596

# Branch name
#ENV GOGS_VERSION v0.9.141
ENV GOGS_VERSION develop


# Install system utils & Gogs runtime dependencies
ADD https://github.com/tianon/gosu/releases/download/1.9/gosu-amd64 /usr/sbin/gosu
RUN chmod +x /usr/sbin/gosu \
 && apk --no-cache --no-progress add ca-certificates bash git linux-pam s6 curl openssh socat tzdata

ENV GOGS_CUSTOM /data/gogs

# Configure LibC Name Service
COPY docker/nsswitch.conf /etc/nsswitch.conf

# Configure Docker Container
VOLUME ["/data"]
EXPOSE 22 3000
ENTRYPOINT ["/app/gogs/docker/start.sh"]
CMD ["/bin/s6-svscan", "/app/gogs/docker/s6/"]



## Build Golang & Gogs


# https://golang.org/issue/14851
#COPY no-pic.patch /


COPY docker /app/gogs/docker

RUN set -ex \
	&& apk add --no-cache --virtual .build-deps \
		bash \
		gcc \
		musl-dev \
		openssl \
		go \
		build-base \
		linux-pam-dev \
	\
	&& export GOROOT_BOOTSTRAP="$(go env GOROOT)" \
	\
	&& wget -q "$GOLANG_SRC_URL" -O golang.tar.gz \
	&& echo "$GOLANG_SRC_SHA256  golang.tar.gz" | sha256sum -c - \
	&& tar -C /usr/local -xzf golang.tar.gz \
	&& rm golang.tar.gz \
	&& cd /usr/local/go/src \
	&& patch -p2 -i /app/gogs/docker/no-pic.patch \
	&& ./make.bash \
	\
	&& rm /app/gogs/docker/*.patch \
	&& git config --global http.https://gopkg.in.followRedirects true \
	&& cd /app/gogs/docker/ \
	&& sh /app/gogs/docker/build.sh \
	&& rm /app/gogs/docker/build.sh \
	&& rm /app/gogs/docker/nsswitch.conf \
	&& rm /app/gogs/docker/README.md \
	&& apk del .build-deps  \
	&& mv /go/src/github.com/gogits/gogs/gogs /app/gogs/ \
	&& mv /go/src/github.com/gogits/gogs/templates /app/gogs/ \
	&& mv /go/src/github.com/gogits/gogs/scripts /app/gogs/ \
	&& mv /go/src/github.com/gogits/gogs/public /app/gogs/ \
	&& rm -rf /go \
	&& rm -rf /usr/local/go