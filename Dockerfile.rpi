FROM hypriot/rpi-alpine-scratch:v3.2
MAINTAINER jp@roemer.im, raxetul@gmail.com

# Install system utils & Gogs runtime dependencies
ADD https://github.com/tianon/gosu/releases/download/1.6/gosu-armhf /usr/sbin/gosu
RUN echo "http://dl-4.alpinelinux.org/alpine/v3.3/main/"      | tee /etc/apk/repositories    \
 && echo "http://dl-4.alpinelinux.org/alpine/v3.3/community/" | tee -a /etc/apk/repositories \
 && echo "@edge http://dl-4.alpinelinux.org/alpine/edge/main" | tee -a /etc/apk/repositories \
 && apk -U --no-progress upgrade \
 && apk -U --no-progress add ca-certificates bash git linux-pam s6@edge curl openssh socat \
 && chmod +x /usr/sbin/gosu

ENV GOGS_CUSTOM /data/gogs

COPY . /app/gogs/
WORKDIR /app/gogs/
RUN ./docker/build.sh

# Configure LibC Name Service
COPY docker/nsswitch.conf /etc/nsswitch.conf

# Configure Docker Container
VOLUME ["/data"]
EXPOSE 22 3000
ENTRYPOINT ["docker/start.sh"]
CMD ["/bin/s6-svscan", "/app/gogs/docker/s6/"]
