package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mmo-server/pkg/rpc"
	pb "mmo-server/proto_define"

	"google.golang.org/grpc"
)

// SimplePlayerServer implements the PlayerService interface
type SimplePlayerServer struct {
	pb.UnimplementedPlayerServiceServer
}

// UpdatePosition implements the PlayerService.UpdatePosition method
func (s *SimplePlayerServer) UpdatePosition(ctx context.Context, req *pb.PlayerPositionRequest) (*pb.PlayerPositionResponse, error) {
	log.Printf("Received position update for player: %s", req.PlayerId)

	// Create a simple response
	resp := &pb.PlayerPositionResponse{
		Success:      true,
		Timestamp:    time.Now().UnixNano() / int64(time.Millisecond),
		ErrorMessage: "",
		CorrectedPosition: &pb.Vector3{
			X: req.Position.X,
			Y: req.Position.Y,
			Z: req.Position.Z,
		},
	}

	return resp, nil
}

func main() {
	// Define command-line flags
	mode := flag.String("mode", "server", "Mode: 'server' or 'client'")
	endpoint := flag.String("endpoint", "localhost:50051", "gRPC endpoint")
	flag.Parse()

	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Starting gRPC test in %s mode, endpoint: %s", *mode, *endpoint)

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	switch *mode {
	case "server":
		// Create a TCP listener
		lis, err := net.Listen("tcp", *endpoint)
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}

		// Create a gRPC server
		grpcServer := grpc.NewServer()

		// Register the PlayerService
		playerService := &SimplePlayerServer{}
		pb.RegisterPlayerServiceServer(grpcServer, playerService)

		// Start the server
		log.Printf("Server started on endpoint: %s", *endpoint)
		log.Println("Press Ctrl+C to stop the server")

		// Start the server in a goroutine
		go func() {
			if err := grpcServer.Serve(lis); err != nil {
				log.Fatalf("Failed to serve: %v", err)
			}
		}()

		// Wait for signal to stop
		<-sigChan
		log.Println("Stopping server...")
		grpcServer.GracefulStop()

	case "client":
		// 使用连接管理器获取客户端
		playerClient, err := rpc.GetPlayerServiceClient(*endpoint)
		if err != nil {
			log.Fatalf("Failed to get player service client: %v", err)
		}
		// 连接由连接管理器管理，不需要在此关闭

		// Create a basic player position request
		request := &pb.PlayerPositionRequest{
			PlayerId: "test-player-1",
			Position: &pb.Vector3{
				X: 100.5,
				Y: 50.2,
				Z: 200.3,
			},
			Rotation: &pb.Vector3{
				X: 0,
				Y: 45.0,
				Z: 0,
			},
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Make RPC call
		log.Println("Sending player position update...")
		response, err := playerClient.UpdatePosition(ctx, request)
		if err != nil {
			log.Fatalf("RPC call failed: %v", err)
		}

		log.Printf("Response received: success=%v, timestamp=%d",
			response.Success, response.Timestamp)

		// Wait for signal to stop
		log.Println("Press Ctrl+C to exit")
		<-sigChan
		log.Println("Client stopped")

		// 在程序结束时清理所有连接
		rpc.DefaultConnManager.CloseAll()
	}
}
