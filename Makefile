.PHONY: build run test

build:
	go build -o bin/dawnlink ./cmd/dawnlink

run: build
	./bin/dawnlink

test:
	go test ./...
