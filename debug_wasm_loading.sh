#!/bin/bash
# Debug script for WASM plugin loading

echo "WASM Plugin Loading Debug"
echo "======================="

# Build with Nix
echo "Building with Nix..."
nix build

if [ $? -ne 0 ]; then
  echo "Build failed! Exiting."
  exit 1
fi

# Set environment variables for maximum debugging
export FLOW_DEBUG_WASM=1
export GOLOG_LEVEL=debug
export GO_DEBUG=1

# Get the path to the result directory
RESULT_PATH=$(readlink -f result)

# Create a special plugins directory for testing
TEST_DIR=$(mktemp -d)
mkdir -p $TEST_DIR/plugins
echo "Created test plugins directory: $TEST_DIR/plugins"

# Copy the WASM module from the flow-processor-latestledger project to our plugins directory
if [ -f "/home/tmosleyiii/projects/obsrvr/flow-processor-latestledger/flow-latest-ledger.wasm" ]; then
  cp "/home/tmosleyiii/projects/obsrvr/flow-processor-latestledger/flow-latest-ledger.wasm" "$TEST_DIR/plugins/"
  echo "Copied flow-latest-ledger.wasm to test plugins directory"
else
  # Alternatively, try to find it in the build output
  find /home/tmosleyiii/projects/obsrvr/flow-processor-latestledger -name "*.wasm" -type f | while read wasm_file; do
    echo "Found WASM file: $wasm_file"
    cp "$wasm_file" "$TEST_DIR/plugins/"
    echo "Copied to test plugins directory"
  done
fi

# Also copy any .so files needed by the pipeline
find plugins -name "*.so" -type f | while read so_file; do
  echo "Found shared library: $so_file"
  cp "$so_file" "$TEST_DIR/plugins/"
  echo "Copied to test plugins directory"
done

# List all files in the plugins directory
echo "Files in plugins directory:"
ls -la $TEST_DIR/plugins/

# Create a test pipeline that explicitly identifies the plugin file
cat > $TEST_DIR/test_pipeline.yaml << EOF
pipelines:
  WasmTestPipeline:
    source:
      type: "BufferedStorageSourceAdapter"
      config:
        bucket_name: "obsrvr-stellar-ledger-data-mainnet-data/landing/"
        network: "mainnet"
        num_workers: 10
        retry_limit: 3
        retry_wait: 5
        start_ledger: 56146570
        ledgers_per_file: 1
        files_per_partition: 64000
    processors:
      - type: "flow/processor/latest-ledger"
        plugin: "flow-latest-ledger.wasm"  # Explicit filename
        config:
          network_passphrase: "Public Global Stellar Network ; September 2015"
    consumers:
      - type: "SaveToZeroMQ"
        config:
          address: "tcp://127.0.0.1:5556"
EOF

echo "Created test pipeline configuration:"
cat $TEST_DIR/test_pipeline.yaml

# Run with verbose debugging, redirecting stderr to stdout for easier reading
echo "Running Flow with debug options..."
$RESULT_PATH/bin/flow \
  -pipeline "$TEST_DIR/test_pipeline.yaml" \
  -instance-id=test \
  -tenant-id=test \
  -api-key=test \
  -plugins="$TEST_DIR/plugins" 2>&1 | grep -E "(wasm|plugin|Initializing|processor|WARNING|ERROR)"

# Show test directory for reference
echo "Test directory: $TEST_DIR" 