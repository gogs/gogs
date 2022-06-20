#!/bin/sh

set -xe

if [ "$(uname -m)" = "aarch64" ]; then
  export arch='arm64'
  export checksum='44fad3d61ad39d0abff33f90fdbb99a666524dbeab08dc9d138d5d3a532ff68a'
elif [ "$(uname -m)" = "armv7l" ]; then
  export arch='arm'
  export checksum='b10ae7d85749025740097b0c349b946fbabd417c7ee4d2df8ccc5604750accd9'
else
  export arch='amd64'
  export checksum='b9c5986f33a53094751b5e22ccc33e050b4a0a485658442121331cbb724e631e'
fi

wget --quiet https://github.com/go-task/task/releases/download/v3.12.1/task_linux_${arch}.tar.gz -O task_linux_${arch}.tar.gz
echo "${checksum}  task_linux_${arch}.tar.gz" | sha256sum -cs

tar -xzf task_linux_${arch}.tar.gz
mv task /usr/local/bin/task
