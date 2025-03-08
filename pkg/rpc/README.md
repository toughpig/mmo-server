# MMO Server RPC Framework

This package provides a high-performance RPC (Remote Procedure Call) framework for inter-process and inter-machine communication in the MMO server infrastructure.

## Features

- **High Performance**: Uses shared memory IPC with shmipc-go for extremely low-latency local communication
- **Protocol Neutrality**: Supports both binary protocol buffer messages and JSON
- **Service Registration**: Automatic registration of service methods using reflection
- **Bidirectional Communication**: Full duplex communication between client and server
- **Contextual Support**: Supports context for cancellation and timeouts
- **Multiple Implementation Options**: 
  - Full-featured implementation using shmipc-go (for production use)
  - Simplified standalone implementation (for testing and examples)

## Usage

### Full Implementation (shmipc-go based)

The primary implementation uses `shmipc-go` for high-performance shared memory communication between processes:

#### Server-side

```go
// Create a new RPC server
server := rpc.NewShmIPCServer("/tmp/my-rpc-server.sock")

// Register a service
myService := &MyService{}
if err := server.Register(myService); err != nil {
    log.Fatalf("Failed to register service: %v", err)
}

// Start the server
if err := server.Start(); err != nil {
    log.Fatalf("Failed to start server: %v", err)
}

// Handle server shutdown gracefully
defer server.Stop()
```

#### Client-side

```go
// Create a new RPC client
client, err := rpc.NewShmIPCClient("/tmp/my-rpc-server.sock")
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}
defer client.Close()

// Make an RPC call
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

request := &MyRequest{...}
response := &MyResponse{}
if err := client.Call(ctx, "MyService.MyMethod", request, response); err != nil {
    log.Fatalf("RPC call failed: %v", err)
}
```

### Simplified Implementation (Standalone)

A simplified version is provided for testing purposes:

#### Server-side

```go
// Start the server (blocking call)
rpc.StartStandaloneServer("/tmp/rpc-standalone.sock")
```

#### Client-side

```go
// Create a client
client, err := rpc.NewSimpleClient("/tmp/rpc-standalone.sock")
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}
defer client.Close()

// Make a call
ctx := context.Background()
response, timestamp, err := client.CallEcho(ctx, "Hello, World!")
if err != nil {
    log.Fatalf("RPC call failed: %v", err)
}
fmt.Printf("Response: %s (at %d)\n", response, timestamp)
```

## Service Implementation

To create a service that can be registered with the RPC server:

```go
// Define your service
type MyService struct{}

// Implement methods (must follow the pattern):
// func (s *Service) Method(ctx context.Context, req *RequestType, resp *ResponseType) error
func (s *MyService) MyMethod(ctx context.Context, req *MyRequest, resp *MyResponse) error {
    // Process the request
    resp.Result = doSomething(req.Input)
    return nil
}
```

## Testing Tools and Configuration

The RPC framework includes comprehensive testing tools with multiple test modes:

### System Requirements

To run the RPC tests that use shmipc-go, your system needs:

- Linux or macOS operating system (shmipc relies on Unix sockets)
- Proper shared memory permissions
- Sufficient shared memory space (sysctl settings on macOS, or /dev/shm on Linux)

### Test Modes

The `rpc_test` tool supports several testing modes:

```bash
# Basic example test - demonstrates server and client setup
./bin/rpc_test -mode=example

# Server mode - starts a standalone RPC server
./bin/rpc_test -mode=server -endpoint=/tmp/my-test-socket.sock

# Client mode - connects to a running server and sends test requests
./bin/rpc_test -mode=client -endpoint=/tmp/my-test-socket.sock

# Stress test - sends many requests sequentially to test performance
./bin/rpc_test -mode=stress -endpoint=/tmp/my-test-socket.sock -requests=100 -interval=10

# Concurrent test - simulates multiple clients accessing the server simultaneously
./bin/rpc_test -mode=concurrent -endpoint=/tmp/my-test-socket.sock -clients=10 -requests=10

# Error handling test - tests how the framework handles various error conditions
./bin/rpc_test -mode=error -endpoint=/tmp/my-test-socket.sock
```

### Command-line Parameters

- `mode`: Test mode (example, server, client, stress, concurrent, error)
- `endpoint`: Unix socket path for IPC
- `requests`: Number of requests to send in stress/concurrent tests
- `clients`: Number of concurrent clients in concurrent test
- `interval`: Delay between requests in milliseconds
- `timeout`: Request timeout in milliseconds

### Troubleshooting Tests

Common issues and solutions:

1. **Socket already in use**:
   ```bash
   rm -f /tmp/my-test-socket.sock
   ```

2. **Protocol initializer timeout**:
   - Increase system shared memory limits
   - Linux: `sudo mount -o remount,size=512M /dev/shm`
   - macOS: `sysctl -w kern.sysv.shmmax=16777216 kern.sysv.shmall=4096`

3. **Permission issues**:
   - Ensure the user has write access to the socket path and shared memory
   - Try running with elevated permissions (sudo) if needed for testing

### Simplified Testing

If you encounter issues with shmipc-go, the standalone implementation provides simpler testing:

```bash
# Run the standalone server (without shmipc dependencies)
./bin/rpc_standalone -mode=server -socket=/tmp/my-test-socket.sock

# Run the standalone client
./bin/rpc_standalone -mode=client -socket=/tmp/my-test-socket.sock -message="Hello, World!"
```

## Implementation Details

The framework consists of several components:

1. **Core RPC Types**: Basic types and interfaces defining the RPC protocol
2. **Service Registry**: Reflection-based mechanism for registering service methods
3. **Protocol Serialization**: Protocol buffer message serialization/deserialization
4. **Transport Layer**: SHMIPC-based communication for high performance
5. **Client Implementation**: Thread-safe client with call tracking and timeouts
6. **Server Implementation**: Multi-connection server handling concurrent requests

## Performance Considerations

- The shmipc-go implementation provides near-zero-copy performance for local process communication
- For optimal performance, ensure services process requests quickly or offload work to separate goroutines
- Consider implementing connection pooling for high-throughput scenarios
- The framework handles automatic reconnection and error recovery 