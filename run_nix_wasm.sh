#!/bin/bash
# Run Flow with WASM support using Nix build

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

# Try to understand what's in the WASM file
echo "WASM file content:"
cat "$WASM_FILE"
echo ""

# Compile and run our WASM test program
echo "Building the WASM loader test program..."
go build -o test_wasm_loader test_wasm_loader.go

if [ $? -ne 0 ]; then
  echo "Failed to build test program!"
  exit 1
fi

echo "Running the WASM loader test program..."
./test_wasm_loader --plugins="$RESULT_PATH/plugins"

if [ $? -ne 0 ]; then
  echo "WASM loader test failed! Stopping here."
  exit 1
fi

# Continue with the full Flow application test
echo "WASM test succeeded! Now running the full Flow application with WASM support..."

# Create a temporary pipeline file with the correct absolute path
TEMP_PIPELINE=$(mktemp)

# Replace the placeholder in the pipeline file
cat examples/pipelines/pipeline_latest_ledger_wasm.yaml | sed "s|DIR_PLACEHOLDER|$RESULT_PATH|g" > $TEMP_PIPELINE

echo "Created temporary pipeline at $TEMP_PIPELINE with content:"
cat $TEMP_PIPELINE

# List the plugins directory to verify files are accessible
echo "Listing plugins directory ($RESULT_PATH/plugins):"
ls -la $RESULT_PATH/plugins/

# Run Flow with our temporary pipeline
echo "Running Flow with WASM pipeline..."
$RESULT_PATH/bin/flow \
  -pipeline $TEMP_PIPELINE \
  -instance-id=1 \
  -tenant-id=1 \
  -api-key=1 \
  -plugins=$RESULT_PATH/plugins

# Clean up
rm $TEMP_PIPELINE
echo "Test completed." 