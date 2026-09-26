GO ?= $(HOME)/.local/go/bin/go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test vet run clean

build:
	$(GO) build -ldflags "-X main.Version=$(VERSION)" -o bin/redditbbs ./cmd/redditbbs

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

run: build
	./bin/redditbbs

clean:
	rm -rf bin
