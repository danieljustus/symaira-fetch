GO ?= go
BINARY_NAME = symfetch

.PHONY: all
all: build test

.PHONY: build
build:
	CGO_ENABLED=0 $(GO) build -ldflags "-s -w -X main.version=dev" -o $(BINARY_NAME) ./cmd/symfetch

.PHONY: build-version
build-version:
	CGO_ENABLED=0 $(GO) build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY_NAME) ./cmd/symfetch

.PHONY: test
test:
	CGO_ENABLED=0 $(GO) test ./...

.PHONY: test-verbose
test-verbose:
	CGO_ENABLED=0 $(GO) test -v ./...

.PHONY: test-race
test-race:
	$(GO) test -race ./...

.PHONY: test-version
test-version:
	$(MAKE) build-version VERSION=ci-test-sentinel
	@OUTPUT=$$(./$(BINARY_NAME) version) && \
	if echo "$$OUTPUT" | grep -q "ci-test-sentinel"; then \
		echo "✓ Version injection OK: $$OUTPUT"; \
	else \
		echo "✗ Version injection FAILED: expected 'ci-test-sentinel', got: $$OUTPUT" >&2; \
		rm -f $(BINARY_NAME); \
		exit 1; \
	fi
	@rm -f $(BINARY_NAME)

.PHONY: lint
lint:
	$(GO) fmt ./...
	CGO_ENABLED=0 $(GO) vet ./...

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

.PHONY: install
install:
	CGO_ENABLED=0 $(GO) install -ldflags "-s -w -X main.version=dev" ./cmd/symfetch
