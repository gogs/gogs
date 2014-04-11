FROM stackbrew/ubuntu:13.10
MAINTAINER  Meaglith Ma <genedna@gmail.com> (@genedna)

ENV DEBIAN_FRONTEND noninteractive

RUN echo "deb http://mirrors.aliyun.com/ubuntu/ saucy main restricted" > /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates main restricted" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-backports main restricted universe multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security main restricted" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security multiverse" >> /etc/apt/sources.list

RUN mkdir -p /go
ENV PATH /usr/local/go/bin:/go/bin:$PATH
ENV GOROOT /usr/local/go
ENV GOPATH /go

RUN apt-get update && apt-get install --yes --force-yes curl git mercurial zip wget ca-certificates build-essential

RUN curl -s http://docker.u.qiniudn.com/go1.2.1.src.tar.gz | tar -v -C /usr/local -xz
RUN cd /usr/local/go/src && ./make.bash --no-clean 2>&1

# You may need a proxy, if github is very slow.
#RUN http_proxy=106.187.38.45:3128 go get -u github.com/gogits/gogs 
RUN go get -u github.com/gogits/gogs 
RUN cd $GOPATH/src/github.com/gogits/gogs && go build

# Clean all the unused packages
RUN apt-get autoremove -y
RUN apt-get clean all

# Add the deploy script to the docker image and assign execution permission to it.
ADD ./deploy.sh /
RUN chmod +x deploy.sh

EXPOSE 3000

CMD /deploy.sh
