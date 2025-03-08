# gRPC Implementation for MMO Server

This document describes the implementation of gRPC in the MMO server framework.

## Overview

The MMO server framework uses gRPC as the primary RPC mechanism. This provides several benefits:

1. Better cross-platform compatibility
2. Improved network transport (TCP)
3. Built-in streaming support
4. Better tooling and ecosystem support
5. More robust error handling

## Implementation Details

The implementation includes:

1. Proto definitions in `proto_define/rpc_service.proto`
2. Message definitions in `proto_define/message.proto` using `google.protobuf.Any`
3. gRPC client implementation in `pkg/rpc/grpc_client.go`
4. gRPC server implementation in `pkg/rpc/grpc_server.go`
5. Factory pattern in `pkg/rpc/factory.go` to make it easy to configure
6. Connection manager in `pkg/rpc/conn_manager.go` for efficient connection reuse

### Connection Management

The framework includes a connection manager that efficiently handles gRPC connections:

- Reuses connections to the same endpoint
- Handles connection pooling
- Provides thread-safe access to connections
- Proper cleanup of connections on shutdown

## Usage

### Creating a Server

```go
// Create a gRPC server
server := rpc.NewRPCServer("localhost:50051", rpc.TransportGRPC)

// Register a service
service := &MyService{}
err := server.Register(service)
if err != nil {
    log.Fatalf("Failed to register service: %v", err)
}

// Start the server
err = server.Start()
if err != nil {
    log.Fatalf("Failed to start server: %v", err)
}
defer server.Stop()
```

### Creating a Client

```go
// Create a gRPC client
client, err := rpc.NewRPCClient("localhost:50051", rpc.TransportGRPC)
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}
defer client.Close()

// Make an RPC call
request := &pb.MyRequest{...}
response := &pb.MyResponse{}
err = client.Call(ctx, "MyService.MyMethod", request, response)
if err != nil {
    log.Fatalf("RPC call failed: %v", err)
}
```

### Direct gRPC Service Access

You can also use direct gRPC service clients for better type safety:

```go
// Get a direct PlayerService client
playerClient, err := rpc.GetPlayerServiceClient("localhost:50051")
if err != nil {
    log.Fatalf("Failed to create player service client: %v", err)
}

// Call the UpdatePosition method directly
posReq := &pb.PlayerPositionRequest{
    PlayerId: "player123",
    Position: &pb.Vector3{
        X: 100.0,
        Y: 0.0,
        Z: 50.0,
    },
}
posResp, err := playerClient.UpdatePosition(ctx, posReq)

// Connection cleanup is handled by the connection manager
// Make sure to call this before your application exits
rpc.DefaultConnManager.CloseAll()
```

## Testing

A test application is provided in `cmd/grpc_test/main.go`. You can build and run it using:

```bash
# Build the test
./build-grpc-test.sh

# Run the server
./bin/grpc-test -mode server

# In another terminal, run the client
./bin/grpc-test -mode client
```

The tests are also integrated into the main test script:

```bash
# Run all tests including gRPC tests
./test.sh
```

## Error Handling

The framework includes comprehensive error handling:

- Connection errors with proper error messages
- Request timeouts and cancellation support
- Automatic retry handling for transient errors
- Proper error propagation from server to client

## Future Improvements

1. Add TLS support for secure communication
2. Implement more advanced gRPC features like interceptors
3. Expand bidirectional streaming capabilities
4. Implement load balancing for multiple servers 