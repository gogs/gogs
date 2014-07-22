FROM stackbrew/ubuntu:13.10
MAINTAINER  Meaglith Ma <genedna@gmail.com> (@genedna)

#aliyun#RUN echo "deb http://mirrors.aliyun.com/ubuntu/ saucy main restricted" > /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates main restricted" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-updates multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-backports main restricted universe multiverse" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security main restricted" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security universe" >> /etc/apt/sources.list && echo "deb http://mirrors.aliyun.com/ubuntu/ saucy-security multiverse" >> /etc/apt/sources.list

#nchc#RUN echo "deb http://free.nchc.org.tw/ubuntu/ saucy main restricted" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy main restricted" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy-updates main restricted" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy-updates main restricted" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy universe" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy universe" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy-updates universe" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy-updates universe" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy multiverse" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy multiverse" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy-updates multiverse" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy-updates multiverse" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy-backports main restricted universe multiverse" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy-backports main restricted universe multiverse" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy-security main restricted" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy-security main restricted" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy-security universe" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy-security universe" >> /etc/apt/source.list && echo "deb http://free.nchc.org.tw/ubuntu/ saucy-security multiverse" >> /etc/apt/source.list && echo "deb-src http://free.nchc.org.tw/ubuntu/ saucy-security multiverse" >> /etc/apt/source.list && echo "deb http://extras.ubuntu.com/ubuntu saucy main" >> /etc/apt/source.list && echo "deb-src http://extras.ubuntu.com/ubuntu saucy main" >> /etc/apt/source.list 

RUN mkdir -p /go
ENV PATH /usr/local/go/bin:/go/bin:$PATH
ENV GOROOT /usr/local/go
ENV GOPATH /go

RUN apt-get update && apt-get install --yes --force-yes curl git mercurial zip wget ca-certificates build-essential
RUN apt-get install -yq vim sudo

RUN curl -s http://docker.u.qiniudn.com/go1.2.1.src.tar.gz | tar -v -C /usr/local -xz
RUN cd /usr/local/go/src && ./make.bash --no-clean 2>&1

RUN go get -u -d github.com/gogits/gogs 
RUN cd $GOPATH/src/github.com/gogits/gogs && git checkout dev && git pull origin dev && go install && go build -tags redis


# Add the deploy script to the docker image and assign execution permission to it.
ADD ./deploy.sh /
RUN chmod +x deploy.sh

EXPOSE 3000

CMD /deploy.sh
