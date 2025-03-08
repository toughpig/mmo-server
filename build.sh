#!/bin/bash

set -e

echo "Building MMO Server Framework components..."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build gateway
echo "Building gateway..."
go build -o bin/gateway cmd/gateway/main.go

# Build protocol test
echo "Building protocol test..."
go build -o bin/protocol_test cmd/protocol_test/main.go

# Build secure WebSocket client
echo "Building secure WebSocket client..."
go build -o bin/secure_ws_client cmd/secure_ws_client/main.go

# Build WebSocket client
echo "Building WebSocket client..."
go build -o bin/ws_client tools/ws_client.go

# Build load test tool
echo "Building load test tool..."
go build -o bin/load_test cmd/load_test/main.go

echo "Build complete! Executables are in the bin/ directory."
echo 
echo "Available components:"
echo "- bin/gateway: The gateway server"
echo "- bin/protocol_test: Protocol conversion and routing test"
echo "- bin/secure_ws_client: Secure WebSocket client for testing"
echo "- bin/ws_client: WebSocket client for testing"
echo "- bin/load_test: Load testing tool" 