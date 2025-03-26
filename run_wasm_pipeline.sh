#!/bin/bash
# Run the Flow app with WASM plugins - test version

echo "Starting Flow with WASM plugin support (TEST MODE)..."

# Build first with Nix to ensure we have the latest binaries
nix build

if [ $? -ne 0 ]; then
  echo "Build failed! Exiting."
  exit 1
fi

# Set environment variables for debugging
export FLOW_DEBUG_WASM=1
export GOLOG_LEVEL=debug

# Get the path to the result directory
RESULT_PATH=$(readlink -f result)

# Print what plugins we have
echo "Available plugins:"
ls -la $RESULT_PATH/plugins/

# Run the Flow application with explicit plugins path
echo "Running Flow with test pipeline..."
$RESULT_PATH/bin/flow \
  -pipeline 'examples/pipelines/test_wasm_pipeline.yaml' \
  -instance-id=test \
  -tenant-id=test \
  -api-key=test \
  -plugins=$RESULT_PATH/plugins 