LDFLAGS += -X "github.com/gogits/gogs/modules/setting.BuildTime=$(shell date -u '+%Y-%m-%d %I:%M:%S %Z')"
LDFLAGS += -X "github.com/gogits/gogs/modules/setting.BuildGitHash=$(shell git rev-parse HEAD)"

TAGS = ""

RELEASE_ROOT = "release"
RELEASE_GOGS = "release/gogs"
NOW = $(shell date -u '+%Y%m%d%I%M%S')

.PHONY: build pack release bindata clean 

build:
	go install -ldflags '$(LDFLAGS)' -tags '$(TAGS)'
	cp '$(GOPATH)/bin/gogs' .

govet:
	go tool vet -composites=false -methods=false -structtags=false .

pack:
	rm -rf $(RELEASE_GOGS)
	mkdir -p $(RELEASE_GOGS)
	cp -r gogs LICENSE README.md README_ZH.md templates public scripts $(RELEASE_GOGS)
	rm -rf $(RELEASE_GOGS)/public/config.codekit $(RELEASE_GOGS)/public/less
	cd $(RELEASE_ROOT) && zip -r gogs.$(NOW).zip "gogs"

release: build pack

bindata: 
	go-bindata -o=modules/bindata/bindata.go -ignore="\\.DS_Store|README.md" -pkg=bindata conf/...

clean:
	go clean -i ./...

clean-mac: clean
	find . -name ".DS_Store" -print0 | xargs -0 rm