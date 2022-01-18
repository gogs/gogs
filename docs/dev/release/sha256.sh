#!/bin/sh

cd gogs || exit # "gogs" is the directory that stores all release archives
for file in *
do
    if [ -f "$file" ]; then
        shasum -a 256 "$file" >> checksum_sha256.txt
    fi
done
