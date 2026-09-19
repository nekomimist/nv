# Nekomimist's Image Viewer - Build Configuration

# Version information (automatically generated from build date)
VERSION := $(shell date +%Y%m%d)
BUILD_DATE := $(shell date '+%Y-%m-%d %H:%M:%S')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
BUILD_TAGS := native_decode
LDFLAGS := -X main.version=$(VERSION) -X 'main.buildDate=$(BUILD_DATE)'
LDFLAGS_GUI := $(LDFLAGS) -H windowsgui
WINDOWS_ARCH ?= amd64
WINDOWS_ZIG ?= zig
ifeq ($(WINDOWS_ARCH),amd64)
WINDOWS_CC := x86_64-w64-mingw32-gcc
WINDOWS_CXX := x86_64-w64-mingw32-g++
WINDOWS_SUFFIX :=
else ifeq ($(WINDOWS_ARCH),arm64)
WINDOWS_CC := $(WINDOWS_ZIG) cc -target aarch64-windows-gnu
WINDOWS_CXX := $(WINDOWS_ZIG) c++ -target aarch64-windows-gnu
WINDOWS_SUFFIX := -arm64
# Zig's external linker needs an explicit GUI subsystem as well.
LDFLAGS_GUI += -extldflags=-Wl,--subsystem,windows
else
$(error Unsupported WINDOWS_ARCH '$(WINDOWS_ARCH)'; use amd64 or arm64)
endif
WINDOWS_ENV := GOOS=windows GOARCH=$(WINDOWS_ARCH) CGO_ENABLED=1 CC="$(WINDOWS_CC)" CXX="$(WINDOWS_CXX)"

# Output binaries
BINARY_LINUX := nv
BINARY_WINDOWS := nv$(WINDOWS_SUFFIX).exe
BINARY_WINDOWS_DEBUG := nv-debug$(WINDOWS_SUFFIX).exe
RESOURCE_FILE := nv_windows_$(WINDOWS_ARCH).syso

# Windows decode dependencies (static mingw-w64 libs, see
# scripts/windows-deps.sh and the windows-deps target below).
LIBDEFLATE_VERSION := 1.24
LIBWEBP_VERSION := 1.5.0

# Default target
.PHONY: all
all: linux windows

# Application builds use CGO-backed PNG/JPEG/WebP decode.
.PHONY: linux
linux: legacy-resource-cleanup
	@echo "Building Linux version v$(VERSION)..."
	CGO_ENABLED=1 go build -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_LINUX)
	@echo "Linux build complete: $(BINARY_LINUX)"

# Windows GUI build
.PHONY: windows
windows: legacy-resource-cleanup $(RESOURCE_FILE) windows-deps
	@echo "Building Windows GUI version v$(VERSION)..."
	$(WINDOWS_ENV) go build -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS_GUI)" -o $(BINARY_WINDOWS)
	@echo "Windows GUI build complete: $(BINARY_WINDOWS)"

.PHONY: windows-arm64
windows-arm64:
	$(MAKE) windows WINDOWS_ARCH=arm64

# Fetch and cross-build libdeflate + libwebp as static mingw-w64 libraries
# for Windows builds (PNG fast path and WebP decode). Existing installations
# are reused; x64 GCC and ARM64 Zig libraries have separate install prefixes.
.PHONY: windows-deps
windows-deps:
	@WINDOWS_ARCH=$(WINDOWS_ARCH) WINDOWS_ZIG="$(WINDOWS_ZIG)" LIBDEFLATE_VERSION=$(LIBDEFLATE_VERSION) LIBWEBP_VERSION=$(LIBWEBP_VERSION) scripts/windows-deps.sh

# Windows debug build (with console)
.PHONY: debug
debug: legacy-resource-cleanup $(RESOURCE_FILE) windows-deps
	@echo "Building Windows debug version v$(VERSION)..."
	$(WINDOWS_ENV) go build -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_WINDOWS_DEBUG)
	@echo "Windows debug build complete: $(BINARY_WINDOWS_DEBUG)"

# Remove the old, architecture-independent generated file before Go scans it.
# Architecture-suffixed resources are selected automatically by the Go tool.
.PHONY: legacy-resource-cleanup
legacy-resource-cleanup:
	@rm -f nv.syso

# Generate Windows resource file from icon
$(RESOURCE_FILE): icon/icon.ico
	@echo "Generating Windows resource file..."
	@if ! command -v rsrc > /dev/null; then \
		echo "Error: rsrc tool not found. Install it with:"; \
		echo "  go install github.com/akavel/rsrc@latest"; \
		exit 1; \
	fi
	rsrc -arch $(WINDOWS_ARCH) -ico icon/icon.ico -o $(RESOURCE_FILE)
	@echo "Resource file generated: $(RESOURCE_FILE)"

# Force icon regeneration
.PHONY: icon
icon: legacy-resource-cleanup
	@echo "Forcing icon regeneration..."
	@rm -f $(RESOURCE_FILE)
	@$(MAKE) $(RESOURCE_FILE)

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_LINUX) nv.exe nv-debug.exe nv-arm64.exe nv-debug-arm64.exe
	@echo "Clean complete"

# Clean everything including generated files
.PHONY: distclean
distclean: clean
	@echo "Cleaning generated files..."
	@rm -f nv.syso nv_windows_amd64.syso nv_windows_arm64.syso
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
	GOCACHE=/tmp/nv-go-build-cache CGO_ENABLED=1 go test -tags $(BUILD_TAGS) ./...

.PHONY: test-pure
test-pure:
	@echo "Running pure tests..."
	GOCACHE=/tmp/nv-go-build-cache go test ./navlogic

.PHONY: test-jxl-large
test-jxl-large:
	@echo "Full-decoding the optional large JPEG XL fixture..."
	NV_TEST_LARGE_JXL=1 GOCACHE=/tmp/nv-go-build-cache go test ./internal/imgdecode -run '^TestDecodeLargeJXLFixture$$' -v -count=1

.PHONY: bench-decode
bench-decode:
	@echo "Benchmarking registered Go image decode..."
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
	GOCACHE=/tmp/nv-go-build-cache CGO_ENABLED=1 go test -tags $(BUILD_TAGS) . -run '^TestGUI'

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Vet code
.PHONY: vet
vet:
	@echo "Vetting code..."
	GOCACHE=/tmp/nv-go-build-cache CGO_ENABLED=1 go vet -tags $(BUILD_TAGS) ./...

# Lint (requires golangci-lint)
.PHONY: lint
lint:
	@echo "Linting code..."
	@if command -v golangci-lint > /dev/null; then \
		GOCACHE=/tmp/nv-go-build-cache CGO_ENABLED=1 golangci-lint run --build-tags $(BUILD_TAGS); \
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
	@echo "  All application builds use CGO native PNG/JPEG/WebP decode."
	@echo "  make           - Build Linux and Windows GUI versions"
	@echo "  make linux     - Build Linux version (nv)"
	@echo "  make windows   - Build Windows GUI version (nv.exe)"
	@echo "  make windows-arm64 - Build Windows ARM64 GUI version with Zig (nv-arm64.exe)"
	@echo "  make debug     - Build Windows console version (nv-debug.exe)"
	@echo "  make debug WINDOWS_ARCH=arm64 - Build ARM64 console version (nv-debug-arm64.exe)"
	@echo "  make all       - Build Linux and Windows GUI versions"
	@echo ""
	@echo "  make icon      - Force regenerate Windows icon resource"
	@echo "  make clean     - Clean build artifacts"
	@echo "  make distclean - Clean everything including generated files"
	@echo ""
	@echo "  make deps      - Install build dependencies"
	@echo "  make windows-deps - Fetch/build libdeflate+libwebp (WINDOWS_ARCH=amd64 or arm64)"
	@echo "  make test      - Run tests with native decode enabled"
	@echo "  make test-pure - Run strict pure/headless-safe tests"
	@echo "  make test-jxl-large - Full-decode the optional large JPEG XL fixture"
	@echo "  make bench-decode - Benchmark registered Go image decoders"
	@echo "  make bench-decode-native - Benchmark native and registered Go image decoders"
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
