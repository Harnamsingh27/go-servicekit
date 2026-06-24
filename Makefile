.PHONY: all lint test test-race cover examples build clean

GO        := go
GOFLAGS   :=
LINT      := golangci-lint
COVER_OUT := coverage.out

all: lint test build

lint:
	$(LINT) run ./...

test:
	$(GO) test $(GOFLAGS) ./...

test-race:
	$(GO) test -race $(GOFLAGS) ./...

cover:
	$(GO) test -race -coverprofile=$(COVER_OUT) -covermode=atomic ./...
	$(GO) tool cover -html=$(COVER_OUT) -o coverage.html

examples:
	$(GO) build ./examples/http-demo/...
	$(GO) build ./examples/grpc-demo/...

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(COVER_OUT) coverage.html
	rm -f bin/*

.DEFAULT_GOAL := all
