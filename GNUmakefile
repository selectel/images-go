default: tests

tests: lint test

build:
	go build ./...

fmt:
	golangci-lint fmt ./...

lint:
	golangci-lint run ./...

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out -covermode count ./...
	go tool cover -func coverage.out

.PHONY: tests build fmt lint test cover
