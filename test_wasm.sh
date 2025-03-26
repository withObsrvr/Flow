#!/bin/bash
# Test the WASM plugin loading directly

# Set environment variables for debugging
export FLOW_DEBUG_WASM=1
export GOLOG_LEVEL=debug

# First build with Nix
echo "Building Flow with Nix..."
nix build 

if [ $? -ne 0 ]; then
  echo "Nix build failed!"
  exit 1
fi

# Get the result path
RESULT_PATH=$(readlink -f result)
echo "Using result path: $RESULT_PATH"

# Make sure the WASM file exists in the Nix build result
WASM_FILE="$RESULT_PATH/plugins/flow-processor-latest-ledger.wasm"
if [ ! -f "$WASM_FILE" ]; then
  echo "ERROR: WASM file not found in Nix build result at $WASM_FILE"
  exit 1
fi

# Build our WASM test tool
echo "Building WASM test tool..."
(cd wasm_test && go mod tidy && go build -o ../wasm_tester)

if [ $? -ne 0 ]; then
  echo "Failed to build WASM test tool!"
  exit 1
fi

# Run the test tool
echo "Running WASM test tool..."
./wasm_tester --plugins="$RESULT_PATH/plugins"

echo "Test completed." 