.PHONY: test lint vet build generate clean help

## help: Show this help message
help:
	@echo "Dome Go SDK"
	@echo ""
	@echo "Usage:"
	@echo "  make test       Run all tests with race detector"
	@echo "  make lint       Run golangci-lint"
	@echo "  make vet        Run go vet"
	@echo "  make build      Build all packages"
	@echo "  make generate   Regenerate protobuf (requires buf)"
	@echo "  make clean      Clean build cache"
	@echo ""

## test: Run all tests with race detector
test:
	go test -race -count=1 ./...

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## vet: Run go vet
vet:
	go vet ./...

## build: Build all packages (compile check)
build:
	go build ./...

## generate: Regenerate protobuf types (requires buf)
generate:
	buf generate

## clean: Clean build cache
clean:
	go clean -cache -testcache
