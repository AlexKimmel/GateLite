APP=gatelite
PKG=./...
BIN=bin/$(APP)

.PHONY: run build test lint fmt tidy docker

run:
	go run ./cmd/$(APP)

build:
	mkdir -p bin
	go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/$(APP)

test:
	go test -race -count=1 $(PKG)

lint:
	golangci-lint run

fmt:
	go fmt $(PKG)

tidy:
	go mod tidy

docker:
	docker build -t $(APP):dev .