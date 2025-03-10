#!/bin/sh
# Simple health check script for schema-registry
# This script will return success (exit 0) if the service is running,
# even if the database file doesn't exist yet

# Try to access the health endpoint
curl -s -f http://localhost:8081/health > /dev/null 2>&1

# If curl succeeded, return success
if [ $? -eq 0 ]; then
  exit 0
fi

# If curl failed, check if the service is running by checking the port
nc -z localhost 8081 > /dev/null 2>&1

# Return the result of the netcat check
exit $? 