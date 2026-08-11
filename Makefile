.PHONY: build install test clean

PREFIX ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo 0.0.0-dev)
BUILD_DATE ?= $(shell date -u +%Y-%m-%d)
LDFLAGS = -s -w \
	-X github.com/grievouz/discoctl/cmd.version=$(VERSION) \
	-X github.com/grievouz/discoctl/cmd.buildDate=$(BUILD_DATE)

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o discoctl .

install: build
	install -d "$(PREFIX)/bin"
	install -m 0755 discoctl "$(PREFIX)/bin/discoctl"

test:
	go test ./...
	go vet ./...

clean:
	go clean
