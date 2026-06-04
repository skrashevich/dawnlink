.PHONY: build run test fmt install-hooks

build:
	go build -o bin/dawnlink ./cmd/dawnlink

run: build
	./bin/dawnlink

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

install-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit
