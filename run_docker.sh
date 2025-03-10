#!/bin/bash

# run_docker.sh - Script to run Flow components using Docker Compose

# Default values
PIPELINE_FILE="/app/examples/pipelines/pipeline_default.yaml"
PIPELINE_DIR="./examples/pipelines"
INSTANCE_ID="docker"
TENANT_ID="docker"
API_KEY="docker-key"
DB_PATH="/app/data/flow_data.db"
BUILD=false
DETACH=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --pipeline)
      # Extract the filename from the path
      PIPELINE_FILENAME=$(basename "$2")
      # Extract the directory from the path and convert to absolute path if needed
      if [[ "$2" == /* ]]; then
        # Absolute path
        PIPELINE_DIR="$2"
      else
        # Relative path - get absolute path
        PIPELINE_DIR="$(cd "$(dirname "$2")" && pwd)"
      fi
      # Set the full path inside the container
      PIPELINE_FILE="/app/pipelines/${PIPELINE_FILENAME}"
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
    --db-path)
      # If it's an absolute path, use it directly
      if [[ "$2" == /* ]]; then
        DB_PATH="$2"
      else
        # If it's a relative path, make it relative to /app/data in the container
        DB_PATH="/app/data/$(basename "$2")"
      fi
      shift 2
      ;;
    --build)
      BUILD=true
      shift
      ;;
    --detach|-d)
      DETACH=true
      shift
      ;;
    --help|-h)
      echo "Usage: $0 [options]"
      echo "Options:"
      echo "  --pipeline PATH     Path to pipeline configuration file (default: examples/pipelines/pipeline_default.yaml)"
      echo "  --instance-id ID    Instance ID (default: $INSTANCE_ID)"
      echo "  --tenant-id ID      Tenant ID (default: $TENANT_ID)"
      echo "  --api-key KEY       API key (default: $API_KEY)"
      echo "  --db-path PATH      Database path (default: flow_data.db)"
      echo "  --build             Build Docker images before starting containers"
      echo "  --detach, -d        Run containers in the background"
      echo "  --help, -h          Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help to see available options"
      exit 1
      ;;
  esac
done

# Export environment variables for docker compose
export PIPELINE_FILE
export PIPELINE_DIR
export INSTANCE_ID
export TENANT_ID
export API_KEY
export DB_PATH

# Print the configuration
echo "Starting Flow with the following configuration:"
echo "  Pipeline file: $PIPELINE_FILE"
echo "  Pipeline directory: $PIPELINE_DIR"
echo "  Instance ID: $INSTANCE_ID"
echo "  Tenant ID: $TENANT_ID"
echo "  API Key: $API_KEY"
echo "  Database path: $DB_PATH"

# Determine the Docker Compose command to use
if command -v docker-compose &> /dev/null; then
  COMPOSE_CMD="docker-compose"
else
  COMPOSE_CMD="docker compose"
fi

# Build and start the containers
if [ "$BUILD" = true ]; then
  if [ "$DETACH" = true ]; then
    $COMPOSE_CMD up --build -d
  else
    $COMPOSE_CMD up --build
  fi
else
  if [ "$DETACH" = true ]; then
    $COMPOSE_CMD up -d
  else
    $COMPOSE_CMD up
  fi
fi

# Function to clean up on exit
cleanup() {
  echo "Shutting down services..."
  $COMPOSE_CMD down
  echo "All services stopped"
  exit 0
}

# Handle termination if not detached
if [ "$DETACH" = false ]; then
  # Handle termination
  trap cleanup SIGINT SIGTERM

  echo "All services started. Press Ctrl+C to stop."
  echo "GraphQL API available at: http://localhost:8080/graphql"
  echo "Schema Registry available at: http://localhost:8081/schema"

  # Wait for user to press Ctrl+C
  wait
else
  echo "Services started in detached mode."
  echo "GraphQL API available at: http://localhost:8080/graphql"
  echo "Schema Registry available at: http://localhost:8081/schema"
  echo "To stop the services, run: $COMPOSE_CMD down"
fi 