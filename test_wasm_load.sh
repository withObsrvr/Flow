#!/bin/bash
# Test script for WASM plugin loading

echo "WASM Plugin Loading Test"
echo "======================="

# Build with Nix to ensure we have the latest binaries
echo "Building with Nix..."
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

# Create a test directory 
TEST_DIR=$(mktemp -d)
echo "Created test directory: $TEST_DIR"

# Copy the WASM file to test directory
mkdir -p $TEST_DIR/plugins
cp $RESULT_PATH/plugins/flow-processor-latest-ledger.wasm $TEST_DIR/plugins/

echo "WASM file copied to test directory"
ls -la $TEST_DIR/plugins/

# Create a minimal test pipeline
cat > $TEST_DIR/test_pipeline.yaml << EOF
pipelines:
  WASMTestPipeline:
    processors:
      - type: "flow/processor/latest-ledger"
        plugin: "flow-processor-latest-ledger.wasm"
        config:
          test_mode: true
EOF

echo "Created test pipeline:"
cat $TEST_DIR/test_pipeline.yaml

# Run the Go WASM tester directly
echo "Running direct WASM test..."
cd wasm_test && go build -o ../wasm_tester && cd ..
./wasm_tester --plugins="$TEST_DIR/plugins"

echo "Direct WASM test completed."
echo "Test directory: $TEST_DIR" 