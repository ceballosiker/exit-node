.PHONY: build test test-integration lint vet install clean

GO ?= go

build:
	$(GO) build -o bin/exitnode ./cmd/exitnode
	$(GO) build -o bin/exitnode-mcp ./cmd/exitnode-mcp

test:
	$(GO) test -race -short ./...

test-integration:
	EXITNODE_INTEGRATION=1 $(GO) test -race -tags integration ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

install:
	$(GO) install ./cmd/exitnode
	$(GO) install ./cmd/exitnode-mcp

clean:
	rm -rf bin/
