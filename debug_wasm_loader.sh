#!/bin/bash
# Debug script for WASM plugin loading

# Set debug env variables
export FLOW_DEBUG_WASM=1
export GOLOG_LEVEL=debug

# Create a temp directory for our test
TEMP_DIR=$(mktemp -d)
echo "Created temp directory: $TEMP_DIR"

# Copy our placeholder WASM file there
mkdir -p "$TEMP_DIR/plugins"
echo "Created plugins directory in $TEMP_DIR"

# Check the content of the existing WASM file
echo "Content of existing WASM file:"
cat plugins/flow-processor-latest-ledger.wasm || echo "WASM file not found"

# Create our custom test WASM file
echo "Creating custom test WASM file..."
echo '{"name":"flow/processor/latest-ledger","version":"1.0.0"}' > "$TEMP_DIR/plugins/flow-processor-latest-ledger.wasm"
chmod +x "$TEMP_DIR/plugins/flow-processor-latest-ledger.wasm"

# Create a temporary pipeline file
TEMP_PIPELINE="$TEMP_DIR/test_pipeline.yaml"
cat examples/pipelines/simple_wasm_test.yaml | sed "s|DIR_PLACEHOLDER|$TEMP_DIR|g" > "$TEMP_PIPELINE"

echo "Created temporary pipeline at $TEMP_PIPELINE with content:"
cat "$TEMP_PIPELINE"

# Run Flow with very minimal arguments
echo "Running Flow with debug options..."
./result/bin/flow \
  -pipeline "$TEMP_PIPELINE" \
  -plugins="$TEMP_DIR/plugins" \
  -instance-id=test \
  -tenant-id=test \
  -api-key=test \
  2>&1 | grep -Ei "(wasm|plugin)"

# Clean up
rm -rf "$TEMP_DIR"
echo "Cleaned up temporary directory" 