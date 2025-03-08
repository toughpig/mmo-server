package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mmo-server/pkg/rpc"
	pb "mmo-server/proto_define"
)

func main() {
	// Define command-line flags
	mode := flag.String("mode", "example", "Mode: 'server', 'client', 'example', 'stress', 'concurrent', 'error'")
	endpoint := flag.String("endpoint", "/tmp/mmo-rpc-test.sock", "IPC endpoint path (Unix socket path)")
	requestCount := flag.Int("requests", 100, "Number of requests to send in stress test mode")
	concurrentClients := flag.Int("clients", 10, "Number of concurrent clients in concurrent test mode")
	interval := flag.Int("interval", 100, "Interval between requests in milliseconds")
	timeout := flag.Int("timeout", 5000, "Request timeout in milliseconds")
	flag.Parse()

	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Starting RPC test in %s mode, endpoint: %s", *mode, *endpoint)

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	switch *mode {
	case "example":
		// Run the example that demonstrates both server and client
		log.Println("Running standard RPC example...")
		rpc.StartExample()

	case "server":
		// Create and start an RPC server
		server := rpc.NewShmIPCServer(*endpoint)

		// Register the example service
		service := &rpc.PlayerService{}
		err := server.Register(service)
		if err != nil {
			log.Fatalf("Failed to register service: %v", err)
		}

		// Remove existing socket file if it exists
		if _, err := os.Stat(*endpoint); err == nil {
			log.Printf("Removing existing socket file: %s", *endpoint)
			if err := os.Remove(*endpoint); err != nil {
				log.Fatalf("Failed to remove existing socket file: %v", err)
			}
		}

		// Start the server
		err = server.Start()
		if err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
		log.Printf("Server started on endpoint: %s", *endpoint)
		log.Println("Press Ctrl+C to stop the server")

		// Wait for signal to stop
		<-sigChan
		log.Println("Stopping server...")
		server.Stop()

	case "client":
		// Create a client
		client, err := rpc.NewShmIPCClient(*endpoint)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

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

		// Create a response object
		response := &pb.PlayerPositionResponse{}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Millisecond)
		defer cancel()

		// Make RPC call
		log.Println("Sending player position update...")
		err = client.Call(ctx, "PlayerService.UpdatePosition", request, response)
		if err != nil {
			log.Fatalf("RPC call failed: %v", err)
		}

		log.Printf("Response received: success=%v, timestamp=%d",
			response.Success, response.Timestamp)

		// Wait for signal to stop
		log.Println("Press Ctrl+C to exit")
		<-sigChan
		log.Println("Client stopped")

	case "stress":
		// Stress test - send multiple requests as fast as possible
		log.Printf("Starting stress test with %d requests...", *requestCount)

		// Create a client
		client, err := rpc.NewShmIPCClient(*endpoint)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		startTime := time.Now()
		successCount := 0
		failCount := 0

		for i := 0; i < *requestCount; i++ {
			// Create request
			request := &pb.PlayerPositionRequest{
				PlayerId:  fmt.Sprintf("player-%d", i),
				Position:  &pb.Vector3{X: float32(i), Y: float32(i * 2), Z: float32(i * 3)},
				Rotation:  &pb.Vector3{X: 0, Y: float32(i), Z: 0},
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			}

			response := &pb.PlayerPositionResponse{}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Millisecond)

			err = client.Call(ctx, "PlayerService.UpdatePosition", request, response)
			cancel()

			if err != nil {
				failCount++
				if i%10 == 0 {
					log.Printf("Request %d failed: %v", i, err)
				}
			} else {
				successCount++
				if i%10 == 0 {
					log.Printf("Request %d succeeded", i)
				}
			}

			// Add a small delay between requests if specified
			if *interval > 0 {
				time.Sleep(time.Duration(*interval) * time.Millisecond)
			}
		}

		duration := time.Since(startTime)
		log.Printf("Stress test completed: %d/%d successful, duration: %v, avg: %v/request",
			successCount, *requestCount, duration, duration/time.Duration(*requestCount))

	case "concurrent":
		// Concurrent test - multiple clients sending requests simultaneously
		log.Printf("Starting concurrent test with %d clients...", *concurrentClients)

		startTime := time.Now()
		successCount := 0
		failCount := 0
		resultsChannel := make(chan bool, *requestCount**concurrentClients)

		// Create multiple clients
		for c := 0; c < *concurrentClients; c++ {
			go func(clientID int) {
				// Create a client
				client, err := rpc.NewShmIPCClient(*endpoint)
				if err != nil {
					log.Printf("Client %d: Failed to create client: %v", clientID, err)
					for i := 0; i < *requestCount; i++ {
						resultsChannel <- false
					}
					return
				}
				defer client.Close()

				for i := 0; i < *requestCount; i++ {
					// Create request
					request := &pb.PlayerPositionRequest{
						PlayerId:  fmt.Sprintf("player-%d-%d", clientID, i),
						Position:  &pb.Vector3{X: float32(i), Y: float32(i * 2), Z: float32(i * 3)},
						Rotation:  &pb.Vector3{X: 0, Y: float32(i), Z: 0},
						Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
					}

					response := &pb.PlayerPositionResponse{}
					ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Millisecond)

					err := client.Call(ctx, "PlayerService.UpdatePosition", request, response)
					cancel()

					if err != nil {
						resultsChannel <- false
						if i%10 == 0 {
							log.Printf("Client %d: Request %d failed: %v", clientID, i, err)
						}
					} else {
						resultsChannel <- true
						if i%10 == 0 {
							log.Printf("Client %d: Request %d succeeded", clientID, i)
						}
					}

					// Add a small delay between requests if specified
					if *interval > 0 {
						time.Sleep(time.Duration(*interval) * time.Millisecond)
					}
				}
			}(c)
		}

		// Collect results
		totalRequests := *requestCount * *concurrentClients
		for i := 0; i < totalRequests; i++ {
			result := <-resultsChannel
			if result {
				successCount++
			} else {
				failCount++
			}

			if i%100 == 0 && i > 0 {
				log.Printf("Progress: %d/%d requests processed", i, totalRequests)
			}
		}

		duration := time.Since(startTime)
		log.Printf("Concurrent test completed: %d/%d successful, duration: %v, avg: %v/request",
			successCount, totalRequests, duration, duration/time.Duration(totalRequests))

	case "error":
		// Error handling test - tests different error conditions
		log.Println("Starting error handling test...")

		// Create a client
		client, err := rpc.NewShmIPCClient(*endpoint)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		// Test 1: Invalid service name
		log.Println("Test 1: Calling non-existent service...")
		err = client.Call(context.Background(), "NonExistentService.Method", &pb.PlayerPositionRequest{}, &pb.PlayerPositionResponse{})
		if err != nil {
			log.Printf("Expected error occurred: %v", err)
		} else {
			log.Println("Error: Expected an error for non-existent service but got success")
		}

		// Test 2: Invalid method name
		log.Println("Test 2: Calling non-existent method...")
		err = client.Call(context.Background(), "PlayerService.NonExistentMethod", &pb.PlayerPositionRequest{}, &pb.PlayerPositionResponse{})
		if err != nil {
			log.Printf("Expected error occurred: %v", err)
		} else {
			log.Println("Error: Expected an error for non-existent method but got success")
		}

		// Test 3: Request timeout
		log.Println("Test 3: Testing request timeout...")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		err = client.Call(ctx, "PlayerService.SlowOperation", &pb.PlayerPositionRequest{}, &pb.PlayerPositionResponse{})
		cancel()
		if err != nil {
			log.Printf("Expected error occurred: %v", err)
		} else {
			log.Println("Error: Expected a timeout error but got success")
		}

		log.Println("Error handling test completed")

	default:
		fmt.Printf("Unknown mode: %s\n", *mode)
		flag.Usage()
		os.Exit(1)
	}

	log.Println("RPC test completed.")
}
