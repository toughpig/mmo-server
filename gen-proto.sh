#!/bin/bash

set -e

echo "Generating Protocol Buffer and gRPC code..."

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "Protocol Buffers compiler (protoc) is not installed."
    echo "Please install it first: https://grpc.io/docs/protoc-installation/"
    exit 1
fi

# Check if protoc-gen-go and protoc-gen-go-grpc are installed
if ! command -v protoc-gen-go &> /dev/null || ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "Protocol Buffers Go plugins are not installed."
    echo "Please install them first:"
    echo "go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
    echo "go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
    exit 1
fi

# Create directory for generated Go files if it doesn't exist
mkdir -p proto_define/gen

# Generate Go code from .proto files
protoc --go_out=. --go_opt=paths=source_relative \
    proto_define/message.proto \
    proto_define/player.proto

# Generate gRPC code
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto_define/rpc_service.proto

echo "Protocol Buffer and gRPC code generated successfully!" 