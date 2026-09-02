BINARY  := port-explorer
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/Viljoen13/port-explorer/cmd.version=$(VERSION)

.PHONY: build install run test lint fmt vet cross clean preview

build: ## Build for the current platform
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: ## Install into $GOPATH/bin
	go install -trimpath -ldflags "$(LDFLAGS)" .

run: build ## Build and launch the dashboard
	./$(BINARY)

test: ## Run the test suite
	go test -race ./...

lint: fmt vet ## Format check + vet on every platform

fmt:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

vet:
	go vet ./...
	GOOS=darwin  go vet ./...
	GOOS=windows go vet ./...

cross: ## Cross-compile release-style binaries into dist/
	@mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe .

preview: ## Render TUI screens to text files for review (no terminal needed)
	@mkdir -p dist/preview
	PREVIEW_OUT=$(CURDIR)/dist/preview go test ./internal/tui -run TestPreview -count=1
	@ls dist/preview

clean:
	rm -rf $(BINARY) $(BINARY).exe dist

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
