package rpc

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "mmo-server/proto_define"
)

// TransportType defines the type of transport to use for RPC
type TransportType string

const (
	// TransportGRPC uses gRPC for RPC transport
	TransportGRPC TransportType = "grpc"
)

var (
	// DefaultTransport is the default transport type to use
	DefaultTransport = TransportGRPC
)

// NewRPCClient creates a new RPC client with the specified transport
func NewRPCClient(endpoint string, transport TransportType) (RPCClient, error) {
	switch transport {
	case TransportGRPC:
		return NewGRPCClient(endpoint)
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", transport)
	}
}

// NewPlayerServiceClient creates a new PlayerService client
func NewPlayerServiceClient(endpoint string) (pb.PlayerServiceClient, error) {
	// Create a gRPC connection
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}

	// 注意：调用者负责关闭连接 (conn.Close())
	// Create a PlayerService client
	client := pb.NewPlayerServiceClient(conn)
	return client, nil
}

// NewMessageServiceClient creates a new MessageService client
func NewMessageServiceClient(endpoint string) (pb.MessageServiceClient, error) {
	// Create a gRPC connection
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}

	// 注意：调用者负责关闭连接 (conn.Close())
	// Create a MessageService client
	client := pb.NewMessageServiceClient(conn)
	return client, nil
}

// NewRPCServer creates a new RPC server with the specified transport
func NewRPCServer(endpoint string, transport TransportType) RPCServer {
	switch transport {
	case TransportGRPC:
		return NewGRPCServer(endpoint)
	default:
		return nil
	}
}

// PrepareEndpoint prepares the endpoint for use
func PrepareEndpoint(endpoint string, transport TransportType) error {
	// For TCP endpoints, check if the port is available
	if transport == TransportGRPC {
		host, port, err := net.SplitHostPort(endpoint)
		if err != nil {
			return fmt.Errorf("invalid endpoint format: %w", err)
		}

		// If host is empty, use localhost
		if host == "" {
			host = "localhost"
		}

		// Try to listen on the port to see if it's available
		lis, err := net.Listen("tcp", fmt.Sprintf("%s:%s", host, port))
		if err != nil {
			return fmt.Errorf("port %s is not available: %w", port, err)
		}

		// If we successfully listened, close it to release the port
		lis.Close()
	}

	return nil
}
