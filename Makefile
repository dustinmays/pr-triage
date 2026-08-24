.PHONY: build test test-race vet lint fmt-check all

build:
	go build -o bin/pr-triage ./cmd/pr-triage

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

all: build vet lint test fmt-check
