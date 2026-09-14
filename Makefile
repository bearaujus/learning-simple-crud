.PHONY: run build test check seed clean

run:
	go run ./cmd/api

build:
	go build -o bin/ ./cmd/api

test:
	go test ./...

check:
	go vet ./...
	go test ./...

seed:
	go run ./cmd/api -seed

clean:
	go clean ./cmd/api
