.PHONY: test test-unit test-integration test-e2e test-all test-cover build

build:
	go build ./...

test: test-unit

test-unit:
	go test -race ./...

test-cover:
	go test -race -cover -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

test-integration:
	go test -race -tags=integration ./...

test-e2e:
	go test -race -tags=e2e ./authz/casdoor/...

test-all:
	go test -race -tags=integration,e2e ./...
