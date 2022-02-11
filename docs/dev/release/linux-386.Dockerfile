# CentOS 7 comes with GLIBC 2.17 which is the most compatible (the lowest)
# version available in a not-too-oudated Linux distribution.
FROM centos:7
RUN yum install --quiet --assumeyes git wget gcc pam-devel zip

# Install Go
RUN wget --quiet https://go.dev/dl/go1.17.7.linux-386.tar.gz -O go.linux-386.tar.gz
RUN sh -c 'echo "5d5472672a2e0252fe31f4ec30583d9f2b320f9b9296eda430f03cbc848400ce go.linux-386.tar.gz" | sha256sum --check --status'
RUN tar -C /usr/local -xzf go.linux-386.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

# Install Task
RUN wget --quiet https://github.com/go-task/task/releases/download/v3.10.0/task_linux_386.tar.gz -O task_linux_386.tar.gz
# RUN sh -c 'echo "90bb2d757f5bf621cf0e7fa24a5da8723025b8a862e3939ce74a888ad8ce1722 task_linux_386.tar.gz" | sha256sum --check --status'
RUN tar -xzf task_linux_386.tar.gz \
  && mv task /usr/local/bin/task

# THIS IS NOT WORKING
# Build bianry (using raw commands because Task release binary for Linux 386 is not running within container)
WORKDIR /gogs.io/gogs
COPY . .
RUN go build -v \
  -ldflags " \
    -X 'gogs.io/gogs/internal/conf.BuildTime=$(date -u "+%Y-%m-%d %I:%M:%S %Z")' \
    -X 'gogs.io/gogs/internal/conf.BuildCommit=$(git rev-parse HEAD)' \
  " \
  -tags 'cert pam' \
  -trimpath -o gogs

# Pack release archives
RUN rm -rf release \
  mkdir -p release/gogs \
  cp -r gogs LICENSE README.md README_ZH.md scripts release/gogs \
  cd release && zip -r gogs.zip gogs
RUN tar -czf gogs.tar.gz gogs
