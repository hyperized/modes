.PHONY: all fmt vet test lint cover build build-aarch64 clean

PKG          := ./...
COVERPROFILE := coverage.out
BINARY       := modes-decode

all: fmt vet test

fmt:
	go fmt $(PKG)

vet:
	go vet $(PKG)

test:
	go test -race -cover -coverprofile=$(COVERPROFILE) $(PKG)

cover: test
	go tool cover -html=$(COVERPROFILE) -o coverage.html

lint:
	golangci-lint run $(PKG)

build:
	go build -o $(BINARY) ./cmd/modes-decode

build-aarch64:
	env GOOS=linux GOARCH=arm64 go build -o $(BINARY)-aarch64 ./cmd/modes-decode

clean:
	rm -f $(COVERPROFILE) coverage.html $(BINARY) $(BINARY)-aarch64
