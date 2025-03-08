#!/bin/bash

set -e

echo "Generating Protocol Buffer code..."

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "Protocol Buffers compiler (protoc) is not installed."
    echo "Please install it first: https://grpc.io/docs/protoc-installation/"
    exit 1
fi

# Create directory for generated Go files if it doesn't exist
mkdir -p proto_define/gen

# Generate Go code from .proto files
protoc --go_out=. --go_opt=paths=source_relative \
    proto_define/message.proto \
    proto_define/player.proto

echo "Protocol Buffer code generated successfully!" 