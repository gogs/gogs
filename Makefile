LDFLAGS += -X "gogs.io/gogs/internal/setting.BuildTime=$(shell date -u '+%Y-%m-%d %I:%M:%S %Z')"
LDFLAGS += -X "gogs.io/gogs/internal/setting.BuildCommit=$(shell git rev-parse HEAD)"

CONF_FILES := $(shell find conf | sed 's/ /\\ /g')
TEMPLATES_FILES := $(shell find templates | sed 's/ /\\ /g')
PUBLIC_FILES := $(shell find public | sed 's/ /\\ /g')
LESS_FILES := $(wildcard public/less/gogs.less public/less/_*.less)
ASSETS_GENERATED := internal/assets/conf/conf_gen.go internal/assets/templates/templates_gen.go internal/assets/public/public_gen.go
GENERATED := $(ASSETS_GENERATED) public/css/gogs.css

OS := $(shell uname)

TAGS = ""
BUILD_FLAGS = "-v"

RELEASE_ROOT = "release"
RELEASE_GOGS = "release/gogs"
NOW = $(shell date -u '+%Y%m%d%I%M%S')
GOVET = go tool vet -composites=false -methods=false -structtags=false

.PHONY: build pack release generate clean

.IGNORE: public/css/gogs.css

all: build

check: test

dist: release

web: build
	./gogs web

govet:
	$(GOVET) gogs.go
	$(GOVET) models pkg routes

build: $(GENERATED)
	go build $(BUILD_FLAGS) -ldflags '$(LDFLAGS)' -tags '$(TAGS)' -trimpath -o gogs

build-dev: $(GENERATED) govet
	go build $(BUILD_FLAGS) -tags '$(TAGS)' -trimpath -o gogs
	cp '$(GOPATH)/bin/gogs' .

build-dev-race: $(GENERATED) govet
	go build $(BUILD_FLAGS) -race -tags '$(TAGS)' -trimpath -o gogs

pack:
	rm -rf $(RELEASE_GOGS)
	mkdir -p $(RELEASE_GOGS)
	cp -r gogs LICENSE README.md README_ZH.md scripts $(RELEASE_GOGS)
	cd $(RELEASE_ROOT) && zip -r gogs.$(NOW).zip "gogs"

release: build pack

generate: $(ASSETS_GENERATED)

internal/assets/conf/conf_gen.go: $(CONF_FILES)
	-rm -f $@
	go generate internal/assets/conf/conf.go
	gofmt -s -w $@

internal/assets/templates/templates_gen.go: $(TEMPLATES_FILES)
	-rm -f $@
	go generate internal/assets/templates/templates.go
	gofmt -s -w $@

internal/assets/public/public_gen.go: $(PUBLIC_FILES)
	-rm -f $@
	go generate internal/assets/public/public.go
	gofmt -s -w $@

less: public/css/gogs.css

public/css/gogs.css: $(LESS_FILES)
	@type lessc >/dev/null 2>&1 && lessc $< >$@ || echo "lessc command not found, skipped."

clean:
	go clean -i ./...

clean-mac: clean
	find . -name ".DS_Store" -print0 | xargs -0 rm

test:
	go test -cover -race ./...

fixme:
	grep -rnw "FIXME" cmd routers models pkg

todo:
	grep -rnw "TODO" cmd routers models pkg

# Legacy code should be remove by the time of release
legacy:
	grep -rnw "LEGACY" cmd routes models pkg
