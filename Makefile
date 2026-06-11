GO ?= go
BINARY_NAME = symfetch
VERSION_PKG = github.com/danieljustus/symaira-fetch/cmd/symfetch

.PHONY: all
all: build test

.PHONY: build
build:
	$(GO) build -ldflags "-s -w -X main.version=dev" -o $(BINARY_NAME) ./cmd/symfetch

.PHONY: build-version
build-version:
	$(GO) build -ldflags "-s -w -X $(VERSION_PKG).version=$(VERSION)" -o $(BINARY_NAME) ./cmd/symfetch

.PHONY: test
test:
	$(GO) test -race ./...

.PHONY: test-verbose
test-verbose:
	$(GO) test -v -race ./...

.PHONY: lint
lint:
	$(GO) fmt ./...
	$(GO) vet ./...

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

.PHONY: install
install:
	$(GO) install -ldflags "-s -w -X $(VERSION_PKG).version=dev" ./cmd/symfetch
