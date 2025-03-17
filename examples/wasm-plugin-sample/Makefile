.PHONY: build clean

# Output WASM file
WASM_FILE = wasm-sample.wasm

# Default target
all: build

# Build the WASM plugin
build:
	@echo "Building WASM plugin..."
	tinygo build -o $(WASM_FILE) -target=wasi ./main.go
	@echo "Build complete: $(WASM_FILE)"

# Copy the built WASM file to the Flow plugins directory
install: build
	@echo "Installing WASM plugin to Flow plugins directory..."
	mkdir -p ../../plugins
	cp $(WASM_FILE) ../../plugins/
	@echo "Installation complete"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(WASM_FILE)
	@echo "Clean complete"

# Help target
help:
	@echo "Available targets:"
	@echo "  all      - Default target, builds the WASM plugin"
	@echo "  build    - Build the WASM plugin"
	@echo "  install  - Build and install the WASM plugin to the Flow plugins directory"
	@echo "  clean    - Remove build artifacts"
	@echo "  help     - Display this help message" 