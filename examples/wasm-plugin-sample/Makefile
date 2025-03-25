.PHONY: build build-go install install-go clean all help

# Output WASM file
WASM_FILE = wasm-sample.wasm
FLOW_PLUGINS_DIR = ../../plugins

# Default target
all: build-go

# Build using TinyGo (requires TinyGo compatible with your Go version)
build-tinygo:
	@echo "Building WASM plugin with TinyGo..."
	tinygo build -o $(WASM_FILE) -target=wasi ./main.go
	@echo "Build complete: $(WASM_FILE)"

# Build with standard Go WASM (compatible with Go 1.24+)
build-go:
	@echo "Building WASM plugin with standard Go..."
	GOOS=js GOARCH=wasm go build -o $(WASM_FILE) ./main.go
	cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" ./
	@echo "Build complete: $(WASM_FILE)"
	@echo "Also copied wasm_exec.js from Go distribution for browser testing"

# For backward compatibility
build: build-go

# Install with TinyGo build
install-tinygo: build-tinygo
	@echo "Installing TinyGo-built WASM plugin to Flow plugins directory..."
	mkdir -p $(FLOW_PLUGINS_DIR)
	cp $(WASM_FILE) $(FLOW_PLUGINS_DIR)/
	@echo "WASM plugin installed to: $(FLOW_PLUGINS_DIR)/$(WASM_FILE)"

# Install with standard Go WASM build
install-go: build-go
	@echo "Installing Go-built WASM plugin to Flow plugins directory..."
	mkdir -p $(FLOW_PLUGINS_DIR)
	cp $(WASM_FILE) $(FLOW_PLUGINS_DIR)/
	cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" $(FLOW_PLUGINS_DIR)/
	@echo "WASM plugin installed to: $(FLOW_PLUGINS_DIR)/$(WASM_FILE)"
	@echo "Also installed wasm_exec.js to: $(FLOW_PLUGINS_DIR)/"

# For backward compatibility
install: install-go

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(WASM_FILE) wasm_exec.js
	@echo "Clean complete"

# Help target
help:
	@echo "Available targets:"
	@echo "  all          - Default target, now builds with standard Go (for Go 1.24 compatibility)"
	@echo "  build-go     - Build the WASM plugin with standard Go"
	@echo "  build-tinygo - Build the WASM plugin with TinyGo (requires compatible TinyGo)"
	@echo "  build        - Alias for build-go"
	@echo "  install-go   - Build with standard Go and install to plugins directory"
	@echo "  install-tinygo - Build with TinyGo and install to plugins directory"
	@echo "  install      - Alias for install-go"
	@echo "  clean        - Remove build artifacts"
	@echo "  help         - Display this help message" 