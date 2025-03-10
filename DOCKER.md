# Running Flow with Docker

This document explains how to run the Flow application using Docker and Docker Compose.

## Prerequisites

- Docker
- Docker Compose

## Quick Start

To quickly start all services with default settings:

```bash
./run_docker.sh
```

This will start the following services:
- Schema Registry on port 8081
- Flow Core service
- GraphQL API on port 8080

## Configuration Options

The `run_docker.sh` script accepts several options to customize the deployment:

```bash
./run_docker.sh --help
```

Options:
- `--pipeline PATH`: Path to pipeline configuration file (default: examples/pipelines/pipeline_default.yaml)
- `--instance-id ID`: Instance ID (default: docker)
- `--tenant-id ID`: Tenant ID (default: docker)
- `--api-key KEY`: API key (default: docker-key)
- `--db-path PATH`: Database path (default: flow_data.db)
- `--build`: Build Docker images before starting containers
- `--detach, -d`: Run containers in the background
- `--help, -h`: Show help message

## Examples

### Build and run with a custom pipeline configuration:

```bash
./run_docker.sh --pipeline examples/pipelines/pipeline_accounts.yaml --build
```

### Run in detached mode:

```bash
./run_docker.sh --detach
```

### Use a custom database path:

```bash
./run_docker.sh --db-path my_custom_database.db
```

## Using Environment Variables

You can also set configuration using environment variables:

```bash
PIPELINE_FILE="/app/pipelines/my_pipeline.yaml" \
PIPELINE_DIR="./my_pipelines" \
INSTANCE_ID="my-instance" \
TENANT_ID="my-tenant" \
API_KEY="my-key" \
DB_PATH="/app/data/my_database.db" \
./run_docker.sh
```

## Running Docker Compose Directly

If you prefer to use Docker Compose directly:

```bash
# Set environment variables
export PIPELINE_FILE="/app/pipelines/my_pipeline.yaml"
export PIPELINE_DIR="./my_pipelines"
export INSTANCE_ID="my-instance"
export TENANT_ID="my-tenant"
export API_KEY="my-key"
export DB_PATH="/app/data/my_database.db"

# Run Docker Compose
docker compose up
```

## Accessing the Services

- GraphQL API: http://localhost:8080/graphql
- Schema Registry: http://localhost:8081/schema

## Stopping the Services

If you started the services in the foreground, press Ctrl+C to stop them.

If you started the services in detached mode, run:

```bash
docker compose down
```

## Data Persistence

Data is stored in the `./data` directory, which is mounted as a volume in the containers. This data persists even after the containers are stopped or removed.

## Custom Pipeline Files

Pipeline files are mounted from the host into the container. By default, the `examples/pipelines` directory is mounted to `/app/pipelines` in the container.

If you specify a custom pipeline file with `--pipeline`, the directory containing that file will be mounted to `/app/pipelines` in the container.

## Custom Database Path

If you specify a relative database path with `--db-path`, it will be created in the `/app/data` directory inside the container.

If you specify an absolute path, it will be used as-is.

## Building the Docker Image Manually

If you want to build the Docker image manually:

```bash
docker build -t obsrvr/flow:latest .
```

## Troubleshooting

### Checking Logs

To check the logs of a specific service:

```bash
docker compose logs schema-registry
docker compose logs flow-core
docker compose logs graphql-api
```

### Accessing a Container Shell

To access a shell in a running container:

```bash
docker compose exec flow-core sh
docker compose exec schema-registry sh
docker compose exec graphql-api sh
```

### Restarting Services

To restart a specific service:

```bash
docker compose restart flow-core
docker compose restart schema-registry
docker compose restart graphql-api
``` 