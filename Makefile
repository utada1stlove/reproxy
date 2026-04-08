APP := reproxy
GO_CACHE := /tmp/reproxy-go-cache
GO_MOD_CACHE := /tmp/reproxy-go-mod-cache

.PHONY: fmt test build

fmt:
	gofmt -w ./cmd ./internal

test:
	GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go test ./...

build:
	mkdir -p bin
	GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go build -o bin/$(APP) ./cmd/reproxy
