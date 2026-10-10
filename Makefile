.PHONY: frontend build test vet release
frontend:
	cd frontend && npm ci --no-audit --no-fund && npm run build
build: frontend
	CGO_ENABLED=0 go build -trimpath -o bin/market-mood ./cmd/market-mood
test: frontend
	CGO_ENABLED=0 go test ./...
vet: frontend
	CGO_ENABLED=0 go vet ./...
release: frontend
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/market-mood-linux-amd64 ./cmd/market-mood
