#!/bin/bash
# Build script for compiling the latest-ledger processor to WASM

echo "Building flow-processor-latest-ledger WASM module..."

# Make sure we're using Go 1.24+ which has WASIP1 support
GO_VERSION=$(go version | grep -o "go1\.[0-9]*")
if [[ "$GO_VERSION" < "go1.21" ]]; then
  echo "Error: This project requires Go 1.21 or later for WASIP1 support"
  echo "Current version: $(go version)"
  exit 1
fi

# Set WASM build environment variables
export GOOS=wasip1
export GOARCH=wasm

# Build the WASM module
echo "Compiling with GOOS=$GOOS GOARCH=$GOARCH"
go build -o flow-processor-latest-ledger.wasm

if [ $? -ne 0 ]; then
  echo "Build failed!"
  exit 1
fi

echo "Build successful: $(pwd)/flow-processor-latest-ledger.wasm"
echo "File size: $(ls -lh flow-processor-latest-ledger.wasm | awk '{print $5}')"

# Optionally copy to your flow plugins directory
if [ -d "../plugins" ]; then
  cp flow-processor-latest-ledger.wasm ../plugins/
  echo "Copied WASM module to ../plugins/"
fi 