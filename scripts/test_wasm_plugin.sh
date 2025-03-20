#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
EXAMPLES_DIR="$PROJECT_ROOT/examples"
WASM_SAMPLE_DIR="$EXAMPLES_DIR/wasm-plugin-sample"
PLUGINS_DIR="$PROJECT_ROOT/plugins"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== Testing WASM Plugin Support ===${NC}"

# Make sure TinyGo is installed
if ! command -v tinygo &> /dev/null; then
    echo -e "${RED}Error: TinyGo is not installed. Please install TinyGo first.${NC}"
    echo "You can add TinyGo to your environment using Nix:"
    echo "  nix develop"
    exit 1
fi

# Build the WASM sample plugin
echo -e "${YELLOW}Building WASM sample plugin...${NC}"
cd "$WASM_SAMPLE_DIR"
if ! make build; then
    echo -e "${RED}Failed to build WASM sample plugin${NC}"
    exit 1
fi
echo -e "${GREEN}WASM sample plugin built successfully${NC}"

# Install the WASM plugin to the plugins directory
echo -e "${YELLOW}Installing WASM plugin to plugins directory...${NC}"
mkdir -p "$PLUGINS_DIR"
cp "$WASM_SAMPLE_DIR/wasm-sample.wasm" "$PLUGINS_DIR/"
echo -e "${GREEN}WASM plugin installed successfully${NC}"

# Run Flow with the WASM sample pipeline
echo -e "${YELLOW}Running Flow with WASM sample pipeline...${NC}"
cd "$PROJECT_ROOT"
./flow run --config examples/pipelines/pipeline_wasm_sample.yaml

echo -e "${GREEN}Test completed successfully!${NC}" 