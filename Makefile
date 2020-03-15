LDFLAGS += -X "gogs.io/gogs/internal/conf.BuildTime=$(shell date -u '+%Y-%m-%d %I:%M:%S %Z')"
LDFLAGS += -X "gogs.io/gogs/internal/conf.BuildCommit=$(shell git rev-parse HEAD)"

CONF_FILES := $(shell find conf | sed 's/ /\\ /g')
TEMPLATES_FILES := $(shell find templates | sed 's/ /\\ /g')
PUBLIC_FILES := $(shell find public | sed 's/ /\\ /g')
LESS_FILES := $(wildcard public/less/*.less)
ASSETS_GENERATED := internal/assets/conf/conf_gen.go internal/assets/templates/templates_gen.go internal/assets/public/public_gen.go
GENERATED := $(ASSETS_GENERATED) public/css/gogs.min.css

TAGS = ""
BUILD_FLAGS = "-v"

RELEASE_ROOT = "release"
RELEASE_GOGS = "release/gogs"
NOW = $(shell date -u '+%Y%m%d%I%M%S')

.PHONY: check dist build build-no-gen pack release generate less clean test fixme todo legacy

.IGNORE: public/css/gogs.css

all: build

check: test

dist: release

web: build
	./gogs web

build: $(GENERATED)
	go build $(BUILD_FLAGS) -ldflags '$(LDFLAGS)' -tags '$(TAGS)' -trimpath -o gogs

build-no-gen:
	go build $(BUILD_FLAGS) -ldflags '$(LDFLAGS)' -tags '$(TAGS)' -trimpath -o gogs

pack:
	rm -rf $(RELEASE_GOGS)
	mkdir -p $(RELEASE_GOGS)
	cp -r gogs LICENSE README.md README_ZH.md scripts $(RELEASE_GOGS)
	cd $(RELEASE_ROOT) && zip -r gogs.$(NOW).zip "gogs"

release: build pack

generate: clean $(ASSETS_GENERATED)

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

less: clean public/css/gogs.min.css

public/css/gogs.min.css: $(LESS_FILES)
	@type lessc >/dev/null 2>&1 && lessc --clean-css --source-map "public/less/gogs.less" $@ || echo "lessc command not found or failed"

clean:
	find . -name "*.DS_Store" -type f -delete

test:
	go test -cover -race ./...

fixme:
	grep -rnw "FIXME" internal

todo:
	grep -rnw "TODO" internal

# Legacy code should be removed by the time of release
legacy:
	grep -rnw "\(LEGACY\|Deprecated\)" internal
