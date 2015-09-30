FROM debian:jessie
MAINTAINER ogarcia@connectical.com

# Update and install required packages
RUN apt-get update -qqy && \
  apt-get install --no-install-recommends -qqy \
    build-essential ca-certificates curl git libpam-dev \
    openssh-server supervisor && \
  apt-get autoclean && \
  apt-get autoremove && \
  rm -rf /var/lib/apt/lists/*

# Use the force
COPY . /gopath/src/github.com/gogits/gogs/

# Build binary and clean up useless files
RUN mkdir -p /app /goroot && \
  curl https://storage.googleapis.com/golang/go1.5.linux-amd64.tar.gz | tar xzf - -C /goroot --strip-components=1 && \
  cd /gopath/src/github.com/gogits/gogs && \
  GOROOT="/goroot" GOPATH="/gopath" PATH="$PATH:/goroot/bin:/gopath/bin" \
    go get -v -tags "sqlite redis memcache cert pam" && \
  GOROOT="/goroot" GOPATH="/gopath" PATH="$PATH:/goroot/bin:/gopath/bin" \
    go build -tags "sqlite redis memcache cert pam" && \
  mv /gopath/src/github.com/gogits/gogs/ /app/gogs/ && \
  rm -r /goroot /gopath

# Create user, fix and setup SSH and prepare data
RUN useradd --shell /bin/bash --system --comment gogits git && \
  mkdir /var/run/sshd && \
  sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd && \
  sed 's@UsePrivilegeSeparation yes@UsePrivilegeSeparation no@' -i /etc/ssh/sshd_config && \
  echo "export VISIBLE=now" >> /etc/profile && \
  echo "PermitUserEnvironment yes" >> /etc/ssh/sshd_config && \
  sed 's@^HostKey@\#HostKey@' -i /etc/ssh/sshd_config && \
  echo "HostKey /data/ssh/ssh_host_key" >> /etc/ssh/sshd_config && \
  echo "HostKey /data/ssh/ssh_host_rsa_key" >> /etc/ssh/sshd_config && \
  echo "HostKey /data/ssh/ssh_host_dsa_key" >> /etc/ssh/sshd_config && \
  echo "HostKey /data/ssh/ssh_host_ecdsa_key" >> /etc/ssh/sshd_config && \
  echo "HostKey /data/ssh/ssh_host_ed25519_key" >> /etc/ssh/sshd_config && \
  echo "export GOGS_CUSTOM=/data/gogs" >> /etc/profile

WORKDIR /app/gogs

ENV GOGS_CUSTOM /data/gogs

EXPOSE 22 3000
VOLUME ["/data"]

CMD ["./docker/start.sh"]
