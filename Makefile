.PHONY: build test test-race vet lint fmt-check prescan-test all agents-sync agents-check

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

prescan-test:
	bash scripts/prescan-test/run.sh
	bash scripts/prescan-test/error-cases.sh

agents-sync:
	go run ./cmd/agent-sync

agents-check:
	go run ./cmd/agent-sync -check

all: build vet lint test fmt-check agents-check
