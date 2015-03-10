FROM google/golang:latest

ENV TAGS="sqlite redis memcache cert" USER="git" HOME="/home/git"

COPY  . /gopath/src/github.com/gogits/gogs/
WORKDIR /gopath/src/github.com/gogits/gogs/

RUN  go get -v -tags="$TAGS" github.com/gogits/gogs \
  && go build -tags="$TAGS" \
  && useradd -d $HOME -m $USER \
  && chown -R $USER .

USER $USER

ENTRYPOINT [ "./gogs" ]

CMD [ "web" ]
