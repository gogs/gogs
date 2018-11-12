FROM golang:alpine AS binarybuilder
# Install build deps
RUN apk --no-cache --no-progress add --virtual build-deps build-base git linux-pam-dev
WORKDIR /go/src/github.com/gogs/gogs
COPY . .
RUN make build TAGS="sqlite cert pam" \
  && wget https://github.com/upx/upx/releases/download/v3.95/upx-3.95-amd64_linux.tar.xz \
  && tar xvJf upx*.tar.xz \
  && chmod +x upx*/upx \
  && ./upx*/upx gogs \
  && rm -rf upx*

FROM alpine:latest
WORKDIR /app/gogs

ENV TZ "Asia/Shanghai"
ENV GOGS_CUSTOM /data/gogs

ADD https://github.com/tianon/gosu/releases/download/1.11/gosu-amd64 /usr/sbin/gosu

# Configure LibC Name Service
COPY docker/nsswitch.conf /etc/nsswitch.conf
COPY docker ./docker
COPY templates ./templates
COPY public ./public
COPY --from=binarybuilder /go/src/github.com/gogs/gogs/gogs .

# Install system utils & Gogs runtime dependencies
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
  tzdata \
  && ln -sf /usr/share/zoneinfo/${TZ} /etc/localtime \
  && echo ${TZ} > /etc/timezone\
  && ./docker/finalize.sh

# Configure Docker Container
VOLUME ["/data"]
EXPOSE 22 3000

ENTRYPOINT ["/app/gogs/docker/start.sh"]
CMD ["/bin/s6-svscan", "/app/gogs/docker/s6/"]
