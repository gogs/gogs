# CentOS 7 comes with GLIBC 2.17 which is the most compatible (the lowest)
# version available in a not-too-oudated Linux distribution.
FROM centos:7
RUN yum install --quiet --assumeyes git wget gcc pam-devel zip

# Install Go
RUN wget --quiet https://go.dev/dl/go1.17.7.linux-amd64.tar.gz -O go.linux-amd64.tar.gz
RUN sh -c 'echo "02b111284bedbfa35a7e5b74a06082d18632eff824fd144312f6063943d49259 go.linux-amd64.tar.gz" | sha256sum --check --status'
RUN tar -C /usr/local -xzf go.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

# Install Task
RUN wget --quiet https://github.com/go-task/task/releases/download/v3.10.0/task_linux_amd64.tar.gz -O task_linux_amd64.tar.gz
RUN sh -c 'echo "f78c861e6c772a3263e478da7ae3223e10c2bc6b7b0728717d30db35d463f4b9 task_linux_amd64.tar.gz" | sha256sum --check --status'
RUN tar -xzf task_linux_amd64.tar.gz \
  && mv task /usr/local/bin/task

# Build bianry
WORKDIR /gogs.io/gogs
COPY . .
RUN TAGS="cert pam" task build

# Pack release archives
RUN task release
RUN tar -C release -czf release/gogs.tar.gz gogs
