.PHONY: all help fmt vet test test-integration lint cover build build-aarch64 clean

PKG          := ./...
COVERPROFILE := coverage.out
BINARY       := modes-decode

all: fmt vet test ## fmt + vet + test (default)

help: ## show this help text
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

fmt: ## format code via golangci-lint formatters (gofmt + gci); falls back to go fmt
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint fmt $(PKG); \
	else \
		echo "golangci-lint not found — falling back to 'go fmt'"; \
		go fmt $(PKG); \
	fi

vet: ## run go vet
	go vet $(PKG)

test: ## run unit tests with -race and coverage
	go test -race -cover -coverprofile=$(COVERPROFILE) $(PKG)

# Integration tests live behind the `integration` build tag so they
# do not run as part of the default `make test`. There are no
# integration tests in the tree today; the target exists so the
# slot is honoured by CI and contributors can add tagged tests
# without changing the Makefile.
test-integration: ## run integration tests (build tag: integration)
	go test -race -tags=integration $(PKG)

cover: test ## render coverage as coverage.html
	go tool cover -html=$(COVERPROFILE) -o coverage.html

lint: ## run golangci-lint
	golangci-lint run $(PKG)

build: ## build the modes-decode CLI for the host platform
	go build -o $(BINARY) ./cmd/modes-decode

build-aarch64: ## cross-build the modes-decode CLI for linux/arm64
	env GOOS=linux GOARCH=arm64 go build -o $(BINARY)-aarch64 ./cmd/modes-decode

clean: ## remove generated artefacts
	rm -f $(COVERPROFILE) coverage.html $(BINARY) $(BINARY)-aarch64
