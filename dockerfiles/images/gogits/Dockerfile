FROM stackbrew/ubuntu:13.10
MAINTAINER  Meaglith Ma <genedna@gmail.com> (@genedna)

ADD ./sources.list /etc/apt/

RUN mkdir -p /go
ENV PATH /usr/local/go/bin:/go/bin:$PATH
ENV GOROOT /usr/local/go
ENV GOPATH /go

RUN apt-get update && apt-get install -y curl git mercurial zip wget ca-certificates build-essential

RUN curl -s http://docker.u.qiniudn.com/go1.2.1.src.tar.gz | tar -v -C /usr/local -xz
RUN cd /usr/local/go/src && ./make.bash --no-clean 2>&1

RUN go get -u github.com/gogits/gogs 
RUN cd $GOPATH/src/github.com/gogits/gogs && go build

RUN apt-get autoremove -y
RUN apt-get clean all
