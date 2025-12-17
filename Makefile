# Burkut - Modern Download Manager
# Named after the golden eagle (berkut) in Turkish mythology
# Makefile for development and build automation

.PHONY: all build build-linux build-linux-arm64 build-linux-arm \
        build-windows build-windows-arm64 build-darwin build-darwin-arm64 \
        build-freebsd build-all-platforms \
        test test-unit test-integration test-cover test-bench lint fmt vet clean \
        run install checksums release version help init

# Variables
BINARY_NAME := burkut
BINARY_DIR := bin
CMD_PATH := ./cmd/burkut
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.date=$(BUILD_TIME) -X main.commit=$(COMMIT)"

# OS/Arch detection
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# Binary suffix (.exe for windows, empty for others)
ifeq ($(GOOS),windows)
    BINARY_SUFFIX := .exe
else
    BINARY_SUFFIX :=
endif

# Binary name with OS/Arch suffix
BINARY := $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(BINARY_SUFFIX)

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# Default target
all: build

# ==================== BUILD TARGETS ====================

# Build for current platform
build:
	@echo "Building $(BINARY_NAME) ($(GOOS)/$(GOARCH))..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY) $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY)"

# ==================== CROSS-COMPILATION ====================
# Note: CGO_ENABLED=0 ensures zero-dependency static binaries

build-linux:
	@echo "Building $(BINARY_NAME) for Linux (amd64)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY_NAME)-linux-amd64"

build-linux-arm64:
	@echo "Building $(BINARY_NAME) for Linux (arm64)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY_NAME)-linux-arm64"

build-linux-arm:
	@echo "Building $(BINARY_NAME) for Linux (arm)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-arm $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY_NAME)-linux-arm"

build-windows:
	@echo "Building $(BINARY_NAME) for Windows (amd64)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY_NAME)-windows-amd64.exe"

build-windows-arm64:
	@echo "Building $(BINARY_NAME) for Windows (arm64)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-windows-arm64.exe $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY_NAME)-windows-arm64.exe"

build-darwin:
	@echo "Building $(BINARY_NAME) for macOS (amd64)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY_NAME)-darwin-amd64"

build-darwin-arm64:
	@echo "Building $(BINARY_NAME) for macOS (arm64/Apple Silicon)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY_NAME)-darwin-arm64"

build-freebsd:
	@echo "Building $(BINARY_NAME) for FreeBSD (amd64)..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-freebsd-amd64 $(CMD_PATH)
	@echo "Created: $(BINARY_DIR)/$(BINARY_NAME)-freebsd-amd64"

build-all-platforms: build-linux build-linux-arm64 build-linux-arm build-windows build-windows-arm64 build-darwin build-darwin-arm64 build-freebsd
	@echo ""
	@echo "All platform binaries built successfully!"
	@echo "Binaries available in $(BINARY_DIR)/"
	@ls -la $(BINARY_DIR)/

# ==================== TESTING ====================

test: test-unit
	@echo "All tests completed"

test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -v -short -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -race -tags=integration ./...

test-cover:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -func=coverage.out

test-bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# ==================== CODE QUALITY ====================

lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	$(GOCMD) mod tidy

vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

check: fmt vet lint test
	@echo "All checks passed"

# ==================== CLEAN ====================

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BINARY_DIR)
	rm -f coverage.out coverage.html
	rm -f $(BINARY_NAME) $(BINARY_NAME).exe
	@echo "Cleaned"

# ==================== RUN & INSTALL ====================

run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_DIR)/$(BINARY)

install: build
	@echo "Installing $(BINARY_NAME) to $(GOPATH)/bin..."
	cp $(BINARY_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY_NAME)$(BINARY_SUFFIX)
	@echo "Installed: $(GOPATH)/bin/$(BINARY_NAME)$(BINARY_SUFFIX)"

# ==================== PROJECT INIT ====================

init:
	@echo "Initializing Burkut project..."
	@if [ ! -f go.mod ]; then \
		$(GOCMD) mod init github.com/kilimcininkoroglu/burkut; \
		echo "Created go.mod"; \
	else \
		echo "go.mod already exists"; \
	fi
	@mkdir -p cmd/burkut internal/cli internal/download internal/engine \
		internal/protocol internal/storage internal/ui internal/hooks pkg/burkut
	@echo "Created directory structure"
	@if [ ! -f cmd/burkut/main.go ]; then \
		echo 'package main\n\nimport "fmt"\n\nvar (\n\tversion = "dev"\n\tdate    = "unknown"\n\tcommit  = "unknown"\n)\n\nfunc main() {\n\tfmt.Printf("Burkut %s (%s) built on %s\\n", version, commit, date)\n}' > cmd/burkut/main.go; \
		echo "Created cmd/burkut/main.go"; \
	fi
	@echo "Burkut project initialized!"

# ==================== RELEASE ====================

checksums:
	@echo "Generating checksums..."
	@cd $(BINARY_DIR) && sha256sum $(BINARY_NAME)-* > checksums.txt
	@echo "Checksums saved to $(BINARY_DIR)/checksums.txt"

release: clean build-all-platforms checksums
	@echo ""
	@echo "Release artifacts ready in $(BINARY_DIR)/"
	@cat $(BINARY_DIR)/checksums.txt

# ==================== DEPENDENCIES ====================

deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) verify

deps-update:
	@echo "Updating dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy

# ==================== GENERATE ====================

generate:
	@echo "Generating code..."
	$(GOCMD) generate ./...

# ==================== VERSION ====================

version:
	@echo "$(BINARY_NAME) build information:"
	@echo "  Version:    $(VERSION)"
	@echo "  Commit:     $(COMMIT)"
	@echo "  Build Time: $(BUILD_TIME)"

# ==================== HELP ====================

help:
	@echo ""
	@echo "Burkut - Modern Download Manager"
	@echo "================================="
	@echo "Named after the golden eagle (berkut) in Turkish mythology"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Project setup:"
	@echo "  init               Initialize project structure and go.mod"
	@echo ""
	@echo "Build targets (native - auto-detects OS/Arch):"
	@echo "  build              Build for current platform"
	@echo "  build-all-platforms Build for all supported platforms"
	@echo ""
	@echo "Cross-compilation targets:"
	@echo "  build-linux        Build for Linux (amd64)"
	@echo "  build-linux-arm64  Build for Linux (arm64)"
	@echo "  build-linux-arm    Build for Linux (arm)"
	@echo "  build-windows      Build for Windows (amd64)"
	@echo "  build-windows-arm64 Build for Windows (arm64)"
	@echo "  build-darwin       Build for macOS (amd64/Intel)"
	@echo "  build-darwin-arm64 Build for macOS (arm64/Apple Silicon)"
	@echo "  build-freebsd      Build for FreeBSD (amd64)"
	@echo ""
	@echo "Test targets:"
	@echo "  test               Run all tests"
	@echo "  test-unit          Run unit tests with coverage"
	@echo "  test-integration   Run integration tests"
	@echo "  test-cover         Run tests with coverage report"
	@echo "  test-bench         Run benchmarks"
	@echo ""
	@echo "Code quality:"
	@echo "  lint               Run golangci-lint"
	@echo "  fmt                Format code and tidy modules"
	@echo "  vet                Run go vet"
	@echo "  check              Run fmt, vet, lint, and test"
	@echo ""
	@echo "Run & Install:"
	@echo "  run                Build and run"
	@echo "  install            Install to GOPATH/bin"
	@echo ""
	@echo "Release:"
	@echo "  release            Build all platforms + checksums"
	@echo "  checksums          Generate SHA256 checksums"
	@echo ""
	@echo "Other:"
	@echo "  deps               Download and verify dependencies"
	@echo "  deps-update        Update dependencies"
	@echo "  generate           Run go generate"
	@echo "  version            Show version info"
	@echo "  clean              Remove build artifacts"
	@echo "  help               Show this help message"
	@echo ""
	@echo "Binary naming: burkut-{os}-{arch}[.exe]"
	@echo "Examples: burkut-linux-amd64, burkut-windows-amd64.exe, burkut-darwin-arm64"
	@echo ""
