#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== Updating Flow to Go 1.24.1 ===${NC}"

# Check Go version
GO_VERSION=$(go version | awk '{print $3}')
echo -e "Current Go version: ${GREEN}$GO_VERSION${NC}"

# Update go.mod
echo -e "${YELLOW}Updating go.mod to Go 1.24.1...${NC}"
# Using sed to update the go version line
sed -i 's/^go .*/go 1.24.1/' go.mod
echo -e "${GREEN}Updated go.mod${NC}"

# Clean and rebuild with Go 1.24.1
echo -e "${YELLOW}Rebuilding project with Go 1.24.1...${NC}"
nix build
echo -e "${GREEN}Project rebuilt with Go 1.24.1${NC}"

# Build WASM plugin with standard Go
echo -e "${YELLOW}Building WASM plugin sample...${NC}"
cd examples/wasm-plugin-sample
make build-go
make install-go
cd ../..
echo -e "${GREEN}WASM plugin built and installed${NC}"

echo -e "${GREEN}=== Go 1.24.1 upgrade complete ===${NC}"
echo -e "You can now run the application with:${YELLOW}"
echo "FLOW_API_PORT=8083 FLOW_API_AUTH_ENABLED=false ./result/bin/flow --api --api-port 8083 --pipeline 'examples/pipelines/pipeline_latest_ledger.yaml' --instance-id=1 --tenant-id=1 --api-key=1"
echo -e "${NC}" 