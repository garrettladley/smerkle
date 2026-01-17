VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X main.version=$(VERSION)

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^//'

.PHONY: confirm
confirm:
	@echo 'Are you sure? [y/N]' && read ans && [ $${ans:-N} = y ]


## install: install dependencies
.PHONY: install
install:
	@make install/go


## install/go: install go dependencies
.PHONY: install/go
install/go:
	@go mod tidy

## test: run tests
.PHONY: test
test:
	@go test -v -race -coverpkg=./... -covermode=atomic -coverprofile=coverage.txt ./... -timeout 5m

## fmt: format code
.PHONY: fmt
fmt:
	@golangci-lint fmt

## lint: run linter
.PHONY: lint
lint:
	@golangci-lint run

## build: build the smerkle binary
.PHONY: build
build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/smerkle ./cmd
