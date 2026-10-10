.DEFAULT_GOAL := build
.PHONY: frontend build test vet release

FRONTEND_SOURCES := $(shell find frontend/src -type f) frontend/index.html frontend/tsconfig.json frontend/vite.config.ts
FRONTEND_PACKAGES := frontend/package.json frontend/package-lock.json

frontend/node_modules/.market-mood-deps: $(FRONTEND_PACKAGES)
	cd frontend && npm ci --no-audit --no-fund
	touch $@

frontend/dist/.build-stamp: frontend/node_modules/.market-mood-deps $(FRONTEND_SOURCES)
	cd frontend && npm run build
	touch $@

frontend: frontend/dist/.build-stamp

build: frontend
	CGO_ENABLED=0 go build -trimpath -o bin/market-mood ./cmd/market-mood

test: frontend
	CGO_ENABLED=0 go test ./...

vet: frontend
	CGO_ENABLED=0 go vet ./...

release: frontend
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/market-mood-linux-amd64 ./cmd/market-mood
