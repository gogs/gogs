# To automake gogs build process.
# Makefile copiedfrom https://github.com/vmware/govmomi project and  modified for gos
.PHONY: test

all: check test

check: goimports govet

goimports:
	@echo checking go imports...
	@! goimports -d . 2>&1 | egrep -v '^$$'

govet:
	@echo checking go vet...
	@go tool vet -structtags=false -methods=false .

test:
	go get
	go test -v $(TEST_OPTS) ./...

build:
	go build -x  github.com/gogits/gogs


install:
	go install github.com/gogits/gogs

clean: 
	rm gogs 
