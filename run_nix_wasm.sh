#!/bin/bash
# Run Flow with WASM support using Nix build

# First build with Nix
echo "Building Flow with Nix..."
nix build 

if [ $? -ne 0 ]; then
  echo "Nix build failed!"
  exit 1
fi

# Create a temporary pipeline file with the correct absolute path
TEMP_PIPELINE=$(mktemp)
RESULT_PATH=$(readlink -f result)  # Get the absolute path to the Nix result
echo "Using result path: $RESULT_PATH"

# Make sure the WASM file exists in the Nix build result
if [ ! -f "$RESULT_PATH/plugins/flow-processor-latest-ledger.wasm" ]; then
  echo "WARNING: WASM file not found in Nix build result at $RESULT_PATH/plugins/"
  echo "WASM file needs to be manually copied:"
  mkdir -p $RESULT_PATH/plugins/
  cp plugins/flow-processor-latest-ledger.wasm $RESULT_PATH/plugins/
  chmod +x $RESULT_PATH/plugins/flow-processor-latest-ledger.wasm
fi

# Replace the placeholder in the pipeline file
cat examples/pipelines/pipeline_latest_ledger_wasm.yaml | sed "s|DIR_PLACEHOLDER|$RESULT_PATH|g" > $TEMP_PIPELINE

echo "Created temporary pipeline at $TEMP_PIPELINE with content:"
cat $TEMP_PIPELINE

# Set environment variables for debugging
export FLOW_DEBUG_WASM=1
export GOLOG_LEVEL=debug

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