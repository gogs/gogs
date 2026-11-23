FROM --platform=$BUILDPLATFORM node:24-alpine3.24@sha256:a34ab6d7f6f4813f4d0c5469d74685604b962fbdbf5793a7bc7b4998ecb867c8 AS webbuilder
RUN corepack enable
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY web ./web
COPY conf/locale ./conf/locale
RUN pnpm install --frozen-lockfile
RUN pnpm --filter gogs-web run build

# Cross-compile on the build platform so that the Go compiler never runs under emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine3.24@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS binarybuilder
RUN apk --no-cache --no-progress add git

WORKDIR /gogs.io/gogs
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=webbuilder /src/public/dist ./public/dist

ARG TARGETOS TARGETARCH TARGETVARIANT
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
  go build -v -trimpath -tags prod \
  -ldflags "-X 'gogs.io/gogs/internal/conf.BuildTime=$(date -u '+%Y-%m-%d %I:%M:%S %Z')' -X 'gogs.io/gogs/internal/conf.BuildCommit=$(git rev-parse HEAD)'" \
  -o .bin/gogs ./cmd/gogs

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
RUN apk --no-cache --no-progress add \
  bash \
  ca-certificates \
  curl \
  git \
  openssh \
  s6 \
  shadow \
  socat \
  tzdata \
  rsync \
  "zlib>1.3.2" \
  "openssl>3.5.7-r0"

ENV GOGS_CUSTOM=/data/gogs
ENV GOGS_LOG_FORMAT=json

# Configure LibC Name Service
COPY docker/nsswitch.conf /etc/nsswitch.conf

WORKDIR /app/gogs
COPY docker ./docker
COPY --from=binarybuilder /gogs.io/gogs/.bin/gogs .

RUN ./docker/build/finalize.sh

# Configure Docker Container
VOLUME ["/data", "/backup"]
EXPOSE 22 3000
# Override when [server] sets another PROTOCOL, HTTP_ADDR or HTTP_PORT, e.g.,
# `docker run -e GOGS_HEALTHCHECK_URL=https://localhost:3000/healthcheck`. The
# certificate is issued for the external domain rather than localhost, and the
# check sends no credentials, so it is not verified.
ENV GOGS_HEALTHCHECK_URL=http://localhost:3000/healthcheck
HEALTHCHECK CMD curl -fksS -o /dev/null --noproxy '*' -- "$GOGS_HEALTHCHECK_URL" || exit 1
ENTRYPOINT ["/app/gogs/docker/start.sh"]
CMD ["/usr/bin/s6-svscan", "/app/gogs/docker/s6/"]
