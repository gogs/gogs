FROM google/golang:latest
MAINTAINER codeskyblue@gmail.com

RUN echo "deb http://ftp.debian.org/debian/ wheezy-backports main" >> /etc/apt/sources.list
RUN apt-get update
RUN apt-get install -y openssh-server rsync libpam-dev

# set the working directory and add current stuff
COPY  . /gopath/src/github.com/gogits/gogs/
WORKDIR /gopath/src/github.com/gogits/gogs/

RUN go get -v -tags "sqlite redis memcache cert pam"
RUN go build -tags "sqlite redis memcache cert pam"

RUN useradd --shell /bin/bash --system --comment gogits git

RUN mkdir /var/run/sshd
# SSH login fix. Otherwise user is kicked off after login
RUN sed 's@session\s*required\s*pam_loginuid.so@session optional pam_loginuid.so@g' -i /etc/pam.d/sshd
RUN sed 's@UsePrivilegeSeparation yes@UsePrivilegeSeparation no@' -i /etc/ssh/sshd_config
RUN echo "export VISIBLE=now" >> /etc/profile
RUN echo "PermitUserEnvironment yes" >> /etc/ssh/sshd_config

# setup server keys on startup
RUN sed 's@^HostKey@\#HostKey@' -i /etc/ssh/sshd_config
RUN echo "HostKey /data/ssh/ssh_host_key" >> /etc/ssh/sshd_config
RUN echo "HostKey /data/ssh/ssh_host_rsa_key" >> /etc/ssh/sshd_config
RUN echo "HostKey /data/ssh/ssh_host_dsa_key" >> /etc/ssh/sshd_config
RUN echo "HostKey /data/ssh/ssh_host_ecdsa_key" >> /etc/ssh/sshd_config
RUN echo "HostKey /data/ssh/ssh_host_ed25519_key" >> /etc/ssh/sshd_config

# prepare data
#ENV USER="git" HOME="/home/git"
ENV GOGS_CUSTOM /data/gogs
RUN echo "export GOGS_CUSTOM=/data/gogs" >> /etc/profile

EXPOSE 22 3000
ENTRYPOINT []
CMD ["./docker/start.sh"]
