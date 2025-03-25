#!/bin/bash
# Build and run Flow with WASM support

# Build Flow with our custom WASM loader changes
echo "Building Flow with WASM support..."
go build -mod=vendor -o bin/flow-wasm cmd/flow/main.go

if [ $? -ne 0 ]; then
  echo "Build failed!"
  exit 1
fi

# Add debug statements to see what's happening with WASM files
export GOLOG_LEVEL=debug
export FLOW_DEBUG_WASM=1  # Custom env var we'll check in our code

# Run Flow with the WASM pipeline
echo "Running Flow with WASM pipeline..."
./bin/flow-wasm \
  -pipeline 'examples/pipelines/pipeline_latest_ledger_wasm.yaml' \
  -instance-id=1 \
  -tenant-id=1 \
  -api-key=1 \
  -plugins=./plugins 