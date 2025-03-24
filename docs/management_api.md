# Flow Management API Documentation

## Overview

The Flow Management API provides HTTP endpoints for controlling and monitoring Flow pipelines. This API allows users to list, start, and stop pipelines, as well as retrieve metrics and check the system's health.

## Enabling the API

The Management API is disabled by default and can be enabled using command-line flags when starting Flow:

```bash
./Flow --api --api-port 8080
```

You can also configure the API server using environment variables:

```bash
export FLOW_API_PORT=8080
export FLOW_API_AUTH_ENABLED=true
export FLOW_API_USERNAME=admin
export FLOW_API_PASSWORD=secure_password
```

## Authentication

The API uses HTTP Basic Authentication by default. To disable authentication, set the `FLOW_API_AUTH_ENABLED` environment variable to `false`.

When authentication is enabled, you must provide a username and password using the `FLOW_API_USERNAME` and `FLOW_API_PASSWORD` environment variables.

## API Endpoints

### Pipeline Management

#### List Pipelines

Returns a list of all available pipelines and their status.

- **URL**: `/api/pipelines`
- **Method**: `GET`
- **Authentication**: Required (if enabled)

**Response Example**:

```json
{
  "success": true,
  "data": [
    {
      "id": "pipeline1",
      "state": "running",
      "current_ledger": 12345,
      "start_time": "2023-08-15T10:30:00Z"
    },
    {
      "id": "pipeline2",
      "state": "stopped"
    },
    {
      "id": "pipeline3",
      "state": "error",
      "last_error": "Failed to connect to data source"
    }
  ]
}
```

#### Start Pipeline

Starts a specific pipeline with optional configuration overrides.

- **URL**: `/api/pipelines/start`
- **Method**: `POST`
- **Authentication**: Required (if enabled)
- **Content-Type**: `application/json`

**Request Body**:

```json
{
  "pipeline_id": "pipeline1",
  "overrides": {
    "start_ledger": 10000,
    "end_ledger": 20000
  }
}
```

The `overrides` field is optional and allows you to override specific configuration values at runtime.

**Response Example**:

```json
{
  "success": true,
  "data": "Pipeline pipeline1 started successfully"
}
```

#### Stop Pipeline

Stops a running pipeline.

- **URL**: `/api/pipelines/stop`
- **Method**: `POST`
- **Authentication**: Required (if enabled)
- **Content-Type**: `application/json`

**Request Body**:

```json
{
  "pipeline_id": "pipeline1"
}
```

**Response Example**:

```json
{
  "success": true,
  "data": "Pipeline pipeline1 stopped successfully"
}
```

### Metrics and Monitoring

#### Get Metrics

Returns system-wide or pipeline-specific metrics.

- **URL**: `/api/metrics`
- **Method**: `GET`
- **Authentication**: Required (if enabled)
- **Query Parameters**: `pipeline_id` (optional) - The ID of the pipeline to get metrics for

**Response Example (System-wide metrics)**:

```json
{
  "success": true,
  "data": {
    "system": {
      "uptime_seconds": 3600,
      "memory_usage_mb": 256,
      "cpu_usage_percent": 15.5
    }
  }
}
```

**Response Example (Pipeline-specific metrics)**:

```json
{
  "success": true,
  "data": {
    "system": {
      "uptime_seconds": 3600,
      "memory_usage_mb": 256,
      "cpu_usage_percent": 15.5
    },
    "pipeline": {
      "id": "pipeline1",
      "state": "running",
      "current_ledger": 12345,
      "start_time": "2023-08-15T10:30:00Z",
      "uptime_seconds": 1800
    }
  }
}
```

#### Health Check

Returns the health status of the Flow instance.

- **URL**: `/api/health`
- **Method**: `GET`
- **Authentication**: Not required

**Response Example**:

```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "version": "1.0.0",
    "uptime": "1h"
  }
}
```

## Response Format

All API endpoints return responses in a consistent JSON format:

```json
{
  "success": true|false,
  "data": <response data>,
  "error": "<error message if success is false>"
}
```

- `success`: Boolean indicating whether the request was successful
- `data`: The response data (only present if success is true)
- `error`: Error message (only present if success is false)

## Error Codes

The API uses standard HTTP status codes:

- `200`: Successful request
- `400`: Bad request (e.g., missing or invalid parameters)
- `401`: Unauthorized (authentication required)
- `404`: Resource not found
- `405`: Method not allowed
- `500`: Internal server error

## Example Usage

### Using cURL

**List Pipelines**:

```bash
curl -X GET -u admin:secure_password http://localhost:8080/api/pipelines
```

**Start Pipeline**:

```bash
curl -X POST -u admin:secure_password -H "Content-Type: application/json" -d '{"pipeline_id": "pipeline1"}' http://localhost:8080/api/pipelines/start
```

**Stop Pipeline**:

```bash
curl -X POST -u admin:secure_password -H "Content-Type: application/json" -d '{"pipeline_id": "pipeline1"}' http://localhost:8080/api/pipelines/stop
```

**Get Metrics**:

```bash
curl -X GET -u admin:secure_password http://localhost:8080/api/metrics?pipeline_id=pipeline1
```

**Health Check**:

```bash
curl -X GET http://localhost:8080/api/health
``` 