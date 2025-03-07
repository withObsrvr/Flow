#!/bin/bash
# run_local.sh - Script to run Flow components locally

# Default values
SCHEMA_REGISTRY_PORT=8081
GRAPHQL_API_PORT=8080
PLUGINS_DIR="./plugins"
PIPELINE_CONFIG="examples/pipelines/pipeline_example.yaml"
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
    --schema-port)
      SCHEMA_REGISTRY_PORT="$2"
      shift 2
      ;;
    --api-port)
      GRAPHQL_API_PORT="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Ensure the binaries are built
echo "Building components..."
go build -buildmode=pie -o bin/flow cmd/flow/main.go
go build -buildmode=pie -o bin/schema-registry cmd/schema-registry/main.go
go build -buildmode=pie -o bin/graphql-api cmd/graphql-api/main.go

# Create a temporary directory for PID files
mkdir -p tmp

# Start Schema Registry in the background
echo "Starting Schema Registry on port $SCHEMA_REGISTRY_PORT..."
bin/schema-registry $SCHEMA_REGISTRY_PORT &
SCHEMA_PID=$!
echo $SCHEMA_PID > tmp/schema-registry.pid

# Wait for Schema Registry to start
sleep 1

# Start Flow with user's configuration
echo "Starting Flow with pipeline config: $PIPELINE_CONFIG..."
bin/flow \
  --instance-id "$INSTANCE_ID" \
  --tenant-id "$TENANT_ID" \
  --api-key "$API_KEY" \
  --plugins "$PLUGINS_DIR" \
  --pipeline "$PIPELINE_CONFIG" &
FLOW_PID=$!
echo $FLOW_PID > tmp/flow.pid

# Wait for Flow to register schemas
sleep 2

# Start GraphQL API service
echo "Starting GraphQL API on port $GRAPHQL_API_PORT..."
bin/graphql-api $GRAPHQL_API_PORT "http://localhost:$SCHEMA_REGISTRY_PORT" &
API_PID=$!
echo $API_PID > tmp/graphql-api.pid

# Function to clean up on exit
cleanup() {
  echo "Shutting down services..."
  
  if [ -f tmp/flow.pid ]; then
    kill $(cat tmp/flow.pid) 2>/dev/null || true
    rm tmp/flow.pid
  fi
  
  if [ -f tmp/schema-registry.pid ]; then
    kill $(cat tmp/schema-registry.pid) 2>/dev/null || true
    rm tmp/schema-registry.pid
  fi
  
  if [ -f tmp/graphql-api.pid ]; then
    kill $(cat tmp/graphql-api.pid) 2>/dev/null || true
    rm tmp/graphql-api.pid
  fi
  
  echo "All services stopped"
  exit 0
}

# Handle termination
trap cleanup SIGINT SIGTERM

echo "All services started. Press Ctrl+C to stop."
echo "GraphQL API available at: http://localhost:$GRAPHQL_API_PORT/graphql"
echo "Schema Registry available at: http://localhost:$SCHEMA_REGISTRY_PORT/schema"

# Wait for all processes
wait 