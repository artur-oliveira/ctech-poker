# Deployed binary MUST be named `app` — CDK userdata expects /opt/app/current/app.
BINARY      := app
BUILD_DIR   := dist
GOOS        ?= linux
GOARCH      ?= arm64
CGO_ENABLED := 0

.PHONY: build lint test vet clean

build:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) ./cmd/server

lint:
	golangci-lint run ./...

test:
	go test ./... -race -coverprofile=coverage.out

vet:
	go vet ./...

clean:
	rm -rf $(BUILD_DIR)
