# Nekomimist's Image Viewer - Build Configuration

# Version information (automatically generated from build date)
VERSION := $(shell date +%Y%m%d)
BUILD_DATE := $(shell date '+%Y-%m-%d %H:%M:%S')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS := -X main.version=$(VERSION) -X 'main.buildDate=$(BUILD_DATE)'
LDFLAGS_GUI := $(LDFLAGS) -H windowsgui

# Output binaries
BINARY_LINUX := nv
BINARY_WINDOWS := nv.exe
BINARY_WINDOWS_DEBUG := nv-debug.exe
BINARY_LINUX_NATIVE := nv-native
BINARY_WINDOWS_NATIVE := nv-native.exe
RESOURCE_FILE := nv.syso

# Default target
.PHONY: all
all: linux windows

# Linux build
.PHONY: linux
linux: $(RESOURCE_FILE)
	@echo "Building Linux version v$(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_LINUX)
	@echo "Linux build complete: $(BINARY_LINUX)"

.PHONY: linux-native
linux-native: $(RESOURCE_FILE)
	@echo "Building Linux native-decode version v$(VERSION)..."
	CGO_ENABLED=1 go build -tags native_decode -ldflags "$(LDFLAGS)" -o $(BINARY_LINUX_NATIVE)
	@echo "Linux native-decode build complete: $(BINARY_LINUX_NATIVE)"

# Windows GUI build
.PHONY: windows
windows: $(RESOURCE_FILE)
	@echo "Building Windows GUI version v$(VERSION)..."
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS_GUI)" -o $(BINARY_WINDOWS)
	@echo "Windows GUI build complete: $(BINARY_WINDOWS)"

.PHONY: windows-native
windows-native: $(RESOURCE_FILE)
	@echo "Building Windows native-decode GUI version v$(VERSION)..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ go build -tags native_decode -ldflags "$(LDFLAGS_GUI)" -o $(BINARY_WINDOWS_NATIVE)
	@echo "Windows native-decode GUI build complete: $(BINARY_WINDOWS_NATIVE)"

# Windows debug build (with console)
.PHONY: debug
debug: $(RESOURCE_FILE)
	@echo "Building Windows debug version v$(VERSION)..."
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_WINDOWS_DEBUG)
	@echo "Windows debug build complete: $(BINARY_WINDOWS_DEBUG)"

# Generate Windows resource file from icon
$(RESOURCE_FILE): icon/icon.ico
	@echo "Generating Windows resource file..."
	@if ! command -v rsrc > /dev/null; then \
		echo "Error: rsrc tool not found. Install it with:"; \
		echo "  go install github.com/akavel/rsrc@latest"; \
		exit 1; \
	fi
	rsrc -ico icon/icon.ico -o $(RESOURCE_FILE)
	@echo "Resource file generated: $(RESOURCE_FILE)"

# Force icon regeneration
.PHONY: icon
icon:
	@echo "Forcing icon regeneration..."
	@rm -f $(RESOURCE_FILE)
	@$(MAKE) $(RESOURCE_FILE)

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_LINUX) $(BINARY_WINDOWS) $(BINARY_WINDOWS_DEBUG) $(BINARY_LINUX_NATIVE) $(BINARY_WINDOWS_NATIVE)
	@echo "Clean complete"

# Clean everything including generated files
.PHONY: distclean
distclean: clean
	@echo "Cleaning generated files..."
	@rm -f $(RESOURCE_FILE)
	@echo "Distclean complete"

# Show build information
.PHONY: info
info:
	@echo "Build Information:"
	@echo "  Version: $(VERSION)"
	@echo "  Build Date: $(BUILD_DATE)"
	@echo "  Git Commit: $(COMMIT)"
	@echo "  LDFLAGS: $(LDFLAGS)"

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing build dependencies..."
	@if ! command -v rsrc > /dev/null; then \
		echo "Installing rsrc..."; \
		go install github.com/akavel/rsrc@latest; \
	fi
	@echo "Dependencies installed"

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	GOCACHE=/tmp/nv-go-build-cache go test ./...

.PHONY: test-pure
test-pure:
	@echo "Running pure tests..."
	GOCACHE=/tmp/nv-go-build-cache go test ./navlogic

.PHONY: bench-decode
bench-decode:
	@echo "Benchmarking stdlib image decode..."
	GOCACHE=/tmp/nv-go-build-cache go test ./internal/imgdecode -run '^$$' -bench '^BenchmarkDecode' -benchmem -count=5

.PHONY: bench-decode-native
bench-decode-native:
	@echo "Benchmarking native image decode..."
	GOCACHE=/tmp/nv-go-build-cache CGO_ENABLED=1 go test ./internal/imgdecode -tags native_decode -run '^$$' -bench '^BenchmarkDecode' -benchmem -count=5

.PHONY: bench-decode-windows
bench-decode-windows:
	@echo "Benchmarking Windows stdlib and WIC decode via WSL..."
	scripts/bench-decode-wsl-windows.sh

.PHONY: test-root-pure
test-root-pure:
	@echo "Running logic-oriented root-package tests..."
	GOCACHE=/tmp/nv-go-build-cache go test . -run '^TestPure'

.PHONY: test-gui
test-gui:
	@echo "Running GUI-dependent tests..."
	GOCACHE=/tmp/nv-go-build-cache go test . -run '^TestGUI'

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Vet code
.PHONY: vet
vet:
	@echo "Vetting code..."
	GOCACHE=/tmp/nv-go-build-cache go vet ./...

# Lint (requires golangci-lint)
.PHONY: lint
lint:
	@echo "Linting code..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, skipping lint"; \
	fi

# Check everything
.PHONY: check
check: fmt vet test lint

# Help
.PHONY: help
help:
	@echo "Nekomimist's Image Viewer - Build Targets:"
	@echo ""
	@echo "  make           - Build Linux and Windows versions"
	@echo "  make linux     - Build Linux version"
	@echo "  make linux-native - Build Linux version with CGO native PNG/JPEG decode"
	@echo "  make windows   - Build Windows GUI version"
	@echo "  make windows-native - Build Windows GUI version with CGO WIC decode"
	@echo "  make debug     - Build Windows debug version (with console)"
	@echo "  make all       - Build all versions"
	@echo ""
	@echo "  make icon      - Force regenerate Windows icon resource"
	@echo "  make clean     - Clean build artifacts"
	@echo "  make distclean - Clean everything including generated files"
	@echo ""
	@echo "  make deps      - Install build dependencies"
	@echo "  make test      - Run tests"
	@echo "  make test-pure - Run strict pure/headless-safe tests"
	@echo "  make bench-decode - Benchmark stdlib PNG/JPEG decode"
	@echo "  make bench-decode-native - Benchmark native PNG/JPEG decode"
	@echo "  make bench-decode-windows - Benchmark Windows stdlib/WIC decode via WSL"
	@echo "  make test-root-pure - Run logic-oriented root-package tests"
	@echo "  make test-gui  - Run GUI-dependent tests"
	@echo "  make fmt       - Format code"
	@echo "  make vet       - Vet code"
	@echo "  make lint      - Lint code (requires golangci-lint)"
	@echo "  make check     - Run all checks (fmt, vet, test, lint)"
	@echo ""
	@echo "  make info      - Show build information"
	@echo "  make help      - Show this help"
