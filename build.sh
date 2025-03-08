#!/bin/bash

set -e

echo "Building MMO Server components..."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build the gateway server
echo "Building gateway server..."
go build -o bin/gateway cmd/gateway/main.go

# Build the main server
echo "Building main server..."
go build -o bin/server cmd/server/main.go

# Build the protocol test tool
echo "Building protocol test tool..."
go build -o bin/protocol_test cmd/protocol_test/main.go

# Build the secure WebSocket client
echo "Building secure WebSocket client..."
go build -o bin/secure_ws_client cmd/secure_ws_client/main.go

# Build the load test tool
echo "Building load test tool..."
go build -o bin/load_test cmd/load_test/main.go

# Build the RPC test tool
echo "Building RPC test tool..."
go build -o bin/rpc_test cmd/rpc_test/main.go

# Build the player state sync test tool
echo "Building player state sync test tool..."
go build -o bin/sync_test cmd/sync_test/main.go

# Build the AOI test tool
echo "Building AOI test tool..."
go build -o bin/aoi_test cmd/aoi_test/main.go

# Show success message and list of binaries
echo "Build completed successfully!"
echo "Binaries available in bin/ directory:"
echo "- bin/gateway: Gateway server"
echo "- bin/server: Main server"
echo "- bin/protocol_test: Protocol test tool"
echo "- bin/secure_ws_client: Secure WebSocket client"
echo "- bin/load_test: Load test tool"
echo "- bin/rpc_test: RPC framework test using shmipc-go"
echo "- bin/sync_test: Player state sync test tool"
echo "- bin/aoi_test: Area Of Interest system test tool"

# Check if binaries are executable
echo "Checking binary permissions..."
chmod +x bin/*
echo "All binaries are now executable." 