.PHONY: build clean test install dev release fmt lint deps help push

# Binary name
BINARY_NAME=pulse
BUILD_DIR=build

# Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build for Linux AMD64
build-linux-amd64:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

# Build for Linux ARM64
build-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .

# Build all platforms
build-all: build-linux-amd64 build-linux-arm64

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)/
	rm -rf dist/

# Run tests
test:
	go test -v ./...

# Install locally
install: build
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Run in development mode
dev: build
	$(BUILD_DIR)/$(BINARY_NAME) agent

# Build release with goreleaser
release:
	goreleaser release --snapshot --clean

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Download dependencies
deps:
	go mod download
	go mod tidy

# Help
help:
	@echo "Available targets:"
	@echo "  build            - Build binary for current platform"
	@echo "  build-linux-amd64 - Build for Linux AMD64"
	@echo "  build-linux-arm64 - Build for Linux ARM64"
	@echo "  build-all        - Build for all platforms"
	@echo "  clean            - Remove build artifacts"
	@echo "  test             - Run tests"
	@echo "  install          - Install binary to /usr/local/bin"
	@echo "  dev              - Build and run in development mode"
	@echo "  release          - Build release with goreleaser"
	@echo "  fmt              - Format code"
	@echo "  lint             - Lint code"
	@echo "  deps             - Download and tidy dependencies"
	@echo "  help             - Show this help message"


push:
	git push origin main && \
	git push --tags