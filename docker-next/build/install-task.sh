#!/bin/sh

set -xe

if [ "$(uname -m)" = "aarch64" ]; then
  export arch='arm64'
  export checksum='17f325293d08f6f964e0530842e9ef1410dd5f83ee6475b493087391032b0cfd'
elif [ "$(uname -m)" = "armv7l" ]; then
  export arch='arm'
  export checksum='e5b0261e9f6563ce3ace9e038520eb59d2c77c8d85f2b47ab41e1fe7cf321528'
else
  export arch='amd64'
  export checksum='a35462ec71410cccfc428072de830e4478bc57a919d0131ef7897759270dff8f'
fi

wget --quiet https://github.com/go-task/task/releases/download/v3.40.1/task_linux_${arch}.tar.gz -O task_linux_${arch}.tar.gz
echo "${checksum}  task_linux_${arch}.tar.gz" | sha256sum -cs

tar -xzf task_linux_${arch}.tar.gz
mv task /usr/local/bin/task
