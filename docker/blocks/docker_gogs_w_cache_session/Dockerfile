FROM golang:latest

RUN useradd -m git

ENV GOGS_PATH $GOPATH/src/github.com/gogits/gogs
ENV GOGS_CUSTOM_CONF_PATH $GOGS_PATH/custom/conf
ENV GOGS_CUSTOM_CONF $GOGS_CUSTOM_CONF_PATH/app.ini

RUN go get github.com/gogits/gogs
# WORKDIR $GOGS_PATH
WORKDIR /go/src/github.com/gogits/gogs
RUN go build github.com/gogits/gogs
RUN chown -R git $GOGS_PATH

ADD init_gogs.sh /tmp/
RUN chown git /tmp/init_gogs.sh
RUN chmod +x /tmp/init_gogs.sh

USER git
ENV HOME /home/git
ENV USER git
ENV PATH $GOGS_PATH:$PATH

ENTRYPOINT ["/tmp/init_gogs.sh"]
CMD ["gogs", "web"]
