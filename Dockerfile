FROM google/debian:wheezy
MAINTAINER u@gogs.io

RUN echo "deb http://ftp.debian.org/debian/ wheezy-backports main" >> /etc/apt/sources.list && \
	apt-get update -qqy && \
	apt-get install --no-install-recommends -qqy \
	curl build-essential ca-certificates git \ 
	openssh-server rsync libpam-dev && \
	apt-get autoclean && \
    apt-get autoremove && \
    rm -rf /var/lib/apt/lists/*

ENV GOROOT /goroot
ENV GOPATH /gopath
ENV PATH $PATH:$GOROOT/bin:$GOPATH/bin

COPY . /gopath/src/github.com/gogits/gogs/
WORKDIR /gopath/src/github.com/gogits/gogs/

# Build binary and clean up useless files
RUN mkdir /goroot && \
	curl https://storage.googleapis.com/golang/go1.5.linux-amd64.tar.gz | tar xzf - -C /goroot --strip-components=1 && \
	go get -v -tags "sqlite redis memcache cert pam" && \
	go build -tags "sqlite redis memcache cert pam" && \
	mkdir /app/ && \
	mv /gopath/src/github.com/gogits/gogs/ /app/gogs/ && \
	rm -r $GOROOT $GOPATH

WORKDIR /app/gogs/

RUN useradd --shell /bin/bash --system --comment gogits git

# SSH login fix, otherwise user is kicked off after login
RUN mkdir /var/run/sshd && \
	sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd && \
	sed 's@UsePrivilegeSeparation yes@UsePrivilegeSeparation no@' -i /etc/ssh/sshd_config && \
	echo "export VISIBLE=now" >> /etc/profile && \
	echo "PermitUserEnvironment yes" >> /etc/ssh/sshd_config

# Setup server keys on startup
RUN sed 's@^HostKey@\#HostKey@' -i /etc/ssh/sshd_config && \
	echo "HostKey /data/ssh/ssh_host_key" >> /etc/ssh/sshd_config && \
	echo "HostKey /data/ssh/ssh_host_rsa_key" >> /etc/ssh/sshd_config && \
	echo "HostKey /data/ssh/ssh_host_dsa_key" >> /etc/ssh/sshd_config && \
	echo "HostKey /data/ssh/ssh_host_ecdsa_key" >> /etc/ssh/sshd_config && \
	echo "HostKey /data/ssh/ssh_host_ed25519_key" >> /etc/ssh/sshd_config

# Prepare data
ENV GOGS_CUSTOM /data/gogs
RUN echo "export GOGS_CUSTOM=/data/gogs" >> /etc/profile

EXPOSE 22 3000
ENTRYPOINT []
CMD ["./docker/start.sh"]