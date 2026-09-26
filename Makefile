GO ?= $(HOME)/.local/go/bin/go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOLANGCI ?= $(shell command -v golangci-lint 2>/dev/null)

.PHONY: build test vet lint run clean

# CGO_ENABLED=0 gives a static binary that runs on any Linux of the same architecture.
build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o bin/redditbbs ./cmd/redditbbs

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint: vet
ifneq ($(GOLANGCI),)
	$(GOLANGCI) run ./...
else
	@echo "golangci-lint not installed; ran go vet only"
endif

run: build
	./bin/redditbbs

clean:
	rm -rf bin
