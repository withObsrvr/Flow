.PHONY: build clean

# Output WASM file
WASM_FILE = wasm-sample.wasm
FLOW_PLUGINS_DIR = ../../plugins

# Default target
all: build

# Build the WASM plugin with TinyGo
build:
	@echo "Building WASM plugin..."
	tinygo build -o $(WASM_FILE) -target=wasi ./main.go
	@echo "Build complete: $(WASM_FILE)"

# Alternative build using standard Go
build-go:
	@echo "Building WASM plugin with standard Go..."
	GOOS=js GOARCH=wasm go build -o $(WASM_FILE) ./main.go
	@echo "Build complete: $(WASM_FILE)"

# Copy the built WASM file to the Flow plugins directory
install: build
	@echo "Installing WASM plugin to Flow plugins directory..."
	mkdir -p $(FLOW_PLUGINS_DIR)
	cp $(WASM_FILE) $(FLOW_PLUGINS_DIR)/
	@echo "WASM plugin installed to: $(FLOW_PLUGINS_DIR)/$(WASM_FILE)"

# Install using standard Go build
install-go: build-go
	@echo "Installing WASM plugin (built with Go) to Flow plugins directory..."
	mkdir -p $(FLOW_PLUGINS_DIR)
	cp $(WASM_FILE) $(FLOW_PLUGINS_DIR)/
	@echo "WASM plugin installed to: $(FLOW_PLUGINS_DIR)/$(WASM_FILE)"

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -f $(WASM_FILE)

# Help target
help:
	@echo "Available targets:"
	@echo "  all      - Default target, builds the WASM plugin"
	@echo "  build    - Build the WASM plugin with TinyGo"
	@echo "  build-go - Build the WASM plugin with standard Go"
	@echo "  install  - Build and install the WASM plugin to the Flow plugins directory"
	@echo "  install-go - Build with Go and install the WASM plugin"
	@echo "  clean    - Remove build artifacts"
	@echo "  help     - Show this help message" 