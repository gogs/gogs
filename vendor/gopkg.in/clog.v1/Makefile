.PHONY: build test vet

build: vet
	go install -v

test:
	mkdir -p test
	go test -v -cover -race -coverprofile=test/coverage.out

vet:
	go vet

coverage:
	go tool cover -html=test/coverage.out