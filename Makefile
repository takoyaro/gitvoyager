BINARY    := gitvoyager
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE      := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS   := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build install uninstall test clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/gitvoyager

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/gitvoyager

uninstall:
	rm -f $(shell go env GOPATH)/bin/$(BINARY)

test:
	go test ./... -v -race

clean:
	rm -rf bin/
