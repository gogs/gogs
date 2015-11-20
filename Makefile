LDFLAGS += -X "github.com/gogits/gogs/modules/setting.BuildTime=$(shell date -u '+%Y-%m-%d %I:%M:%S %Z')"
LDFLAGS += -X "github.com/gogits/gogs/modules/setting.BuildGitHash=$(shell git rev-parse HEAD)"

DATA_FILES ?= $(shell find conf | sed 's/ /\\ /g')
LESS_FILES ?= $(wildcard public/less/gogs.less public/less/_*.less)

GENERATED ?= modules/bindata/bindata.go public/css/gogs.css

TAGS ?=

DIST := dist
BIN := bin
FILES := templates public scripts LICENSE README.md README_ZH.md

RELEASES ?= $(DIST)/gogs-linux-amd64.tgz \
  $(DIST)/gogs-linux-386.tgz \
  $(DIST)/gogs-linux-arm.tgz \
  $(DIST)/gogs-darwin-amd64.tgz \
  $(DIST)/gogs-darwin-386.tgz \
  $(DIST)/gogs-darwin-arm.tgz \
  $(DIST)/gogs-freebsd-amd64.tgz \
  $(DIST)/gogs-freebsd-386.tgz \
  $(DIST)/gogs-freebsd-arm.tgz \
  $(DIST)/gogs-openbsd-amd64.tgz \
  $(DIST)/gogs-openbsd-386.tgz \
  $(DIST)/gogs-openbsd-arm.tgz \
  $(DIST)/gogs-windows-amd64.zip \
  $(DIST)/gogs-windows-386.zip

.PHONY: clean test deps gofmt govet build install
.IGNORE: public/css/gogs.css

clean:
	go clean -i ./...
	rm -rf $(BIN) $(DIST)

test:
	@echo "Tests are not integrated!" # go test -tags '$(TAGS)' -cover ./...

deps:
	go get -tags '$(TAGS)' -d -t ./...

gofmt:
	go fmt ./...

govet:
	go tool vet -composites=false -methods=false -structtags=false .

build: install
	cp $(GOPATH)/bin/gogs . 

install:
	go install -ldflags '$(LDFLAGS)' -tags '$(TAGS)'

bindata: modules/bindata/bindata.go

modules/bindata/bindata.go: $(DATA_FILES)
	go-bindata -o=$@ -ignore="\\.DS_Store|README.md" -pkg=bindata conf/...

less: public/css/gogs.css

public/css/gogs.css: $(LESS_FILES)
	lessc $< $@

release: $(RELEASES)

$(BIN)/%/gogs/gogs: GOOS=$(firstword $(subst -, ,$*))
$(BIN)/%/gogs/gogs: GOARCH=$(subst .exe,,$(word 2,$(subst -, ,$*)))
$(BIN)/%/gogs/gogs:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags '$(LDFLAGS)' -tags '$(TAGS)' -o $@
	cp -r $(FILES) $(BIN)/$*/gogs

$(DIST)/gogs-%.tgz: GOOS=$(firstword $(subst -, ,$*))
$(DIST)/gogs-%.tgz: GOARCH=$(subst .exe,,$(word 2,$(subst -, ,$*)))
$(DIST)/gogs-%.tgz: $(BIN)/%/gogs/gogs
	mkdir -p $(DIST)
	tar -czf $@ --directory=$(BIN)/$* gogs

$(DIST)/gogs-%.zip: GOOS=$(firstword $(subst -, ,$*))
$(DIST)/gogs-%.zip: GOARCH=$(subst .exe,,$(word 2,$(subst -, ,$*)))
$(DIST)/gogs-%.zip: $(BIN)/%/gogs/gogs
	@mkdir -p $(DIST)
	(cd $(BIN)/$* && zip -r - gogs) > $@
