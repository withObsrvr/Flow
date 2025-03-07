#!/bin/bash
# run_original.sh - Script to run the original Flow binary

# Default values
PLUGINS_DIR="./plugins"
PIPELINE_CONFIG="pipeline_example.yaml"
INSTANCE_ID="local-dev"
TENANT_ID="local"
API_KEY="local-dev-key"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --pipeline)
      PIPELINE_CONFIG="$2"
      shift 2
      ;;
    --plugins)
      PLUGINS_DIR="$2"
      shift 2
      ;;
    --instance-id)
      INSTANCE_ID="$2"
      shift 2
      ;;
    --tenant-id)
      TENANT_ID="$2"
      shift 2
      ;;
    --api-key)
      API_KEY="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Run the original Flow binary
echo "Running original Flow binary with pipeline config: $PIPELINE_CONFIG..."
./Flow \
  --instance-id "$INSTANCE_ID" \
  --tenant-id "$TENANT_ID" \
  --api-key "$API_KEY" \
  --plugins "$PLUGINS_DIR" \
  --pipeline "$PIPELINE_CONFIG" 