FROM --platform=$BUILDPLATFORM node:24-alpine AS webbuilder
RUN corepack enable
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY web ./web
COPY conf/locale ./conf/locale
RUN pnpm install --frozen-lockfile
RUN pnpm --filter gogs-web run build

FROM golang:1.26-alpine3.23 AS binarybuilder
RUN apk --no-cache --no-progress add --virtual \
  build-deps \
  build-base \
  git \
  linux-pam-dev

WORKDIR /gogs.io/gogs
COPY . .
COPY --from=webbuilder /src/public/dist ./public/dist

RUN ./docker/build/install-task.sh
RUN TAGS="pam prod" task build

FROM alpine:3.23
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
  rsync \
  "zlib>1.3.2"

ENV GOGS_CUSTOM=/data/gogs

# Configure LibC Name Service
COPY docker/nsswitch.conf /etc/nsswitch.conf

WORKDIR /app/gogs
COPY docker ./docker
COPY --from=binarybuilder /gogs.io/gogs/.bin/gogs .

RUN ./docker/build/finalize.sh

# Configure Docker Container
VOLUME ["/data", "/backup"]
EXPOSE 22 3000
HEALTHCHECK CMD (curl --noproxy localhost -o /dev/null -sS http://localhost:3000/healthcheck) || exit 1
ENTRYPOINT ["/app/gogs/docker/start.sh"]
CMD ["/usr/bin/s6-svscan", "/app/gogs/docker/s6/"]
