#!/bin/bash -
#

if ! test -d /data/gogs
then
	mkdir -p /var/run/sshd
	mkdir -p /data/gogs/data /data/gogs/conf /data/gogs/log /data/git
fi

if ! test -d /data/ssh
then
	mkdir /data/ssh
	ssh-keygen -q -f /data/ssh/ssh_host_key -N '' -t rsa1
	ssh-keygen -q -f /data/ssh/ssh_host_rsa_key -N '' -t rsa
	ssh-keygen -q -f /data/ssh/ssh_host_dsa_key -N '' -t dsa
	ssh-keygen -q -f /data/ssh/ssh_host_ecdsa_key -N '' -t ecdsa
	ssh-keygen -q -f /data/ssh/ssh_host_ed25519_key -N '' -t ed25519
	chown -R root:root /data/ssh/*
	chmod 600 /data/ssh/*
fi

service ssh start

# sync templates
test -d /data/gogs/templates || cp -ar ./templates /data/gogs/
rsync -rtv /data/gogs/templates/ ./templates/

ln -sf /data/gogs/log ./log
ln -sf /data/gogs/data ./data
ln -sf /data/git /home/git


if ! test -d ~git/.ssh
then
  mkdir ~git/.ssh
  chmod 700 ~git/.ssh
fi

if ! test -f ~git/.ssh/environment
then
  echo "GOGS_CUSTOM=/data/gogs" > ~git/.ssh/environment
  chown git:git ~git/.ssh/environment
  chown 600 ~git/.ssh/environment
fi

chown -R git:git /data .
exec su git -c "./gogs web"
