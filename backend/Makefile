.PHONY: run build tidy test

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -o bin/wybo-server ./cmd/server

tidy:
	go mod tidy

test:
	go test ./...
