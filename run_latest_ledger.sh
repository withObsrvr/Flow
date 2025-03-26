#!/bin/bash
set -e

export FLOW_DEBUG=1
export FLOW_METRICS_ADDR=:8080

echo "Starting Flow with latest ledger processor..."
./Flow \
  -pipeline /tmp/tmp.8VJNC4xb5F/simple_pipeline.yaml \
  -plugins ./plugins \
  -instance-id "test-latest-ledger" \
  -tenant-id "test" \
  -api-key "dummy-api-key"
