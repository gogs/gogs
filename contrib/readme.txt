Steps: 
 1. make sure rpmbuild environment in your home is setup.
    go get -u -tags "sqlite tidb pam cert" github.com/gogits/gogs
    go build  -x tags "sqlite tidb pam cert"  .
 2. genereate gogs binary at gogs top directory.
 3. (cd contrib;make)
