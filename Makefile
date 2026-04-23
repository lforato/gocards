BINARY  := gocards
CMD     := ./cmd/gocards
DIST    := dist
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w

.PHONY: all build install run test fmt vet tidy clean release help

all: build

## build: Compile the binary for the current platform (./gocards)
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

## install: Install gocards into $GOBIN (or $GOPATH/bin)
install:
	go install -ldflags "$(LDFLAGS)" $(CMD)

## run: Build, then launch the TUI
run: build
	./$(BINARY)

## test: Run the full test suite
test:
	go test ./...

## fmt: Format every Go file in the module
fmt:
	gofmt -s -w .

## vet: Run go vet across the module
vet:
	go vet ./...

## tidy: Run go mod tidy
tidy:
	go mod tidy

## clean: Remove build artifacts
clean:
	rm -rf $(BINARY) $(DIST)

## release: Cross-compile release binaries into ./dist for common platforms
release: clean
	@mkdir -p $(DIST)
	@for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do \
		os=$${target%/*}; arch=$${target#*/}; \
		ext=$$(if [ "$$os" = "windows" ]; then echo .exe; fi); \
		out="$(DIST)/$(BINARY)-$(VERSION)-$$os-$$arch$$ext"; \
		echo "==> $$out"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -ldflags "$(LDFLAGS)" -o "$$out" $(CMD) || exit 1; \
	done

## help: Print available targets
help:
	@awk -F':' '/^## / { sub(/^## /, ""); printf "  \033[36m%-10s\033[0m %s\n", $$1, substr($$0, index($$0, ":") + 2) }' $(MAKEFILE_LIST)
