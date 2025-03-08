#!/bin/bash

set -e

echo "Building gRPC test..."

# Create bin directory if it doesn't exist
mkdir -p bin

# Build the gRPC test
go build -o bin/grpc-test cmd/grpc_test/main.go

echo "Build completed. Run with:"
echo "./bin/grpc-test -mode server"
echo "or"
echo "./bin/grpc-test -mode client" 