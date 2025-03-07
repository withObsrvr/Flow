#!/bin/bash

# Exit on any error
set -e

echo "Building Flow application and plugins..."

# Function to update all dependencies in current directory
update_dependencies() {
    echo "Updating all dependencies in $(pwd)"
    
    # Get list of all direct dependencies
    echo "Getting list of dependencies..."
    deps=$(go list -m all | tail -n +2 | cut -d' ' -f1)
    
    # Try to update each dependency individually
    for dep in $deps; do
        echo "Attempting to update $dep..."
        go get -u "$dep" || echo "Warning: Could not update $dep, skipping..."
    done
    
    echo "Running go mod tidy..."
    go mod tidy || echo "Warning: go mod tidy had some issues, continuing..."
}

# Function to verify all dependency versions
verify_versions() {
    echo "Verifying all dependency versions in $(pwd)"
    go list -m all  # Lists all dependencies and their versions
}

# Update and build main application
echo "Updating and building main application..."
cd ~/projects/obsrvr/flow
update_dependencies
verify_versions
go build -buildmode=pie -o Flow

# Update and build source plugin
echo "Updating and building source plugin..."
cd ../flow-source-bufferedstorage-gcs
update_dependencies
verify_versions
go build -buildmode=plugin -o ../flow/plugins/flow-source-bufferedstorage-gcs.so

# Update and build processor plugin
echo "Updating and building processor plugin..."
cd ../flow-processor-latestledger
update_dependencies
verify_versions
go build -buildmode=plugin -o ../flow/plugins/flow-processor-latestledger.so

# Update and build processor plugin 
echo "Updating and building processor plugin..."
cd ../flow-processor-contract-events
update_dependencies
verify_versions
go build -buildmode=plugin -o ../flow/plugins/flow-processor-contract-events.so

# Update and build processor plugin
echo "Updating and building processor plugin..."
cd ../flow-processor-kale-metrics
update_dependencies
verify_versions
go build -buildmode=plugin -o ../flow/plugins/flow-processor-kale-metrics.so

# Update and build consumer plugin
echo "Updating and building consumer plugin..."
cd ../flow-consumer-zeromq
update_dependencies
verify_versions
go build -buildmode=plugin -o ../flow/plugins/flow-consumer-zeromq.so



echo "Build complete!" 