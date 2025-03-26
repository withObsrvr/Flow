#!/bin/bash
# Script to implement the WASM plugin workaround

echo "WASM Plugin Workaround"
echo "======================"

# Set environment variables for debugging
export FLOW_DEBUG_WASM=1
export GOLOG_LEVEL=debug

# Get the test directory
TEST_DIR="/tmp/tmp.8VJNC4xb5F"
if [ ! -d "$TEST_DIR" ]; then
  echo "Test directory not found. Running connect_wasm_pluginmanager.sh first..."
  ./connect_wasm_pluginmanager.sh
  TEST_DIR=$(ls -td /tmp/tmp.* | head -1)
fi

echo "Using test directory: $TEST_DIR"

# Make sure the plugins directory exists
mkdir -p plugins
echo "Ensuring plugins directory exists: $(pwd)/plugins"

# Copy the WASM module to the plugins directory
cp "$TEST_DIR/plugins/flow-latest-ledger.wasm" plugins/
echo "Copied flow-latest-ledger.wasm to plugins directory"

# Now build with Nix to ensure we have the latest binaries
echo "Building with Nix..."
nix build

if [ $? -ne 0 ]; then
  echo "Build failed! Exiting."
  exit 1
fi

# Get the path to the result directory
RESULT_PATH=$(readlink -f result)

# Create a wrapper script for the flow binary in the current directory
cat > run_latest_ledger.sh << EOF
#!/bin/bash
# Wrapper script for running the latest ledger pipeline

# Set environment variables for debugging
export FLOW_DEBUG_WASM=1
export GOLOG_LEVEL=debug

# Run Flow with the simple pipeline
$RESULT_PATH/bin/flow \\
  -pipeline '$TEST_DIR/simple_pipeline.yaml' \\
  -instance-id=test \\
  -tenant-id=test \\
  -api-key=test \\
  -plugins=./plugins \\
  "\$@"
EOF

chmod +x run_latest_ledger.sh

echo ""
echo "==================================="
echo "WASM Plugin Workaround is Ready!"
echo "==================================="
echo "To run Flow with the latest ledger pipeline, use:"
echo "  ./run_latest_ledger.sh"
echo ""
echo "Remember that this workaround doesn't use the 'plugin' field in the pipeline YAML."
echo "Instead, it registers the processor under its type name: 'flow/processor/latest-ledger'"
echo "" 