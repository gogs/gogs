FROM golang:alpine3.21 AS binarybuilder
RUN apk --no-cache --no-progress add --virtual \
  build-deps \
  build-base \
  git \
  linux-pam-dev

WORKDIR /gogs.io/gogs
COPY . .

RUN ./docker/build/install-task.sh
RUN TAGS="cert pam" task build

FROM alpine:3.21

# Create git user and group with fixed UID/GID at build time for better K8s security context support.
# Using 1000:1000 as it's a common non-root UID/GID that works well with most volume permission setups.
ARG GOGS_UID=1000
ARG GOGS_GID=1000
RUN addgroup -g ${GOGS_GID} -S git && \
    adduser -u ${GOGS_UID} -G git -H -D -g 'Gogs Git User' -h /data/git -s /bin/bash git

RUN apk --no-cache --no-progress add \
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
  rsync

ENV GOGS_CUSTOM=/data/gogs

# Configure LibC Name Service
COPY docker/nsswitch.conf /etc/nsswitch.conf

WORKDIR /app/gogs
COPY docker ./docker
COPY --from=binarybuilder /gogs.io/gogs/gogs .

RUN ./docker/build/finalize.sh

# Set ownership of application directory to git user
RUN chown -R git:git /app/gogs

# Configure Docker Container
VOLUME ["/data", "/backup"]
EXPOSE 22 3000
HEALTHCHECK CMD (curl -o /dev/null -sS http://localhost:3000/healthcheck) || exit 1

# Run as non-root user by default for better K8s security context support.
# Note: If you need SSH server functionality, you may need to run as root or use privileged mode.
USER git:git

ENTRYPOINT ["/app/gogs/docker/start.sh"]
CMD ["/usr/bin/s6-svscan", "/app/gogs/docker/s6/"]
