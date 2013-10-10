#!/usr/bin/env bash

if [ ! -f install.sh ]; then
echo 'build.sh must be run within its container folder' 1>&2
exit 1
fi 

CURDIR=`pwd`
NEWPATH="$GOPATH/src/github.com/gpmgo/gopm"
if [ ! -d "$NEWPATH" ]; then
ln -s $CURDIR $NEWPATH 
fi

gofmt -w $CURDIR

cd $NEWPATH
go install
cd $CURDIR

echo 'Build successfully!'
