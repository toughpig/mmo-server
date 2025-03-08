package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// SimpleEchoService is a very basic service for testing our RPC framework
type SimpleEchoService struct{}

// Echo simply echoes back the received message with a timestamp
func (s *SimpleEchoService) Echo(ctx context.Context, req *EchoArgs, resp *EchoReply) error {
	log.Printf("Echo called with message: %s", req.Message)
	resp.Message = req.Message
	resp.Time = time.Now().UnixNano() / int64(time.Millisecond)
	return nil
}

// StartStandaloneServer runs a standalone RPC server with the echo service
func StartStandaloneServer(socketPath string) {
	log.Printf("Starting standalone server on %s", socketPath)

	// Remove existing socket file if it exists
	if _, err := os.Stat(socketPath); err == nil {
		log.Printf("Removing existing socket file: %s", socketPath)
		if err := os.Remove(socketPath); err != nil {
			log.Fatalf("Failed to remove existing socket file: %v", err)
		}
	}

	// Create TCP listener (not using shmipc for simplicity)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on unix socket %s: %v", socketPath, err)
	}
	defer listener.Close()

	// Create service
	service := &SimpleEchoService{}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start accepting connections
	log.Printf("RPC server started. Listening on %s", socketPath)

	// Handle connections in a separate goroutine
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Error accepting connection: %v", err)
				continue
			}

			log.Printf("Connection accepted from %s", conn.RemoteAddr())
			go handleConnection(conn, service)
		}
	}()

	// Wait for signal to terminate
	<-sigChan
	log.Println("Shutting down RPC server...")
}

// handleConnection processes RPC requests for a single connection
func handleConnection(conn net.Conn, service *SimpleEchoService) {
	defer conn.Close()

	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			log.Printf("Error reading from connection: %v", err)
			return
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buffer[:n])

			// Process the request in a separate goroutine
			go func() {
				response, err := handleRequest(data, service)
				if err != nil {
					log.Printf("Error processing request: %v", err)

					// Create an error response
					errResp := SimpleJsonResponse{
						ServiceMethod: "",
						Sequence:      0,
						Error:         err.Error(),
					}

					respBytes, _ := json.Marshal(errResp)
					conn.Write(respBytes)
					return
				}

				// Send the response
				_, err = conn.Write(response)
				if err != nil {
					log.Printf("Error sending response: %v", err)
					return
				}
			}()
		}
	}
}

// handleRequest processes an RPC request and returns a response
func handleRequest(data []byte, service *SimpleEchoService) ([]byte, error) {
	// Parse the JSON request
	var req SimpleJsonRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request format: %w", err)
	}

	// Check if we support this method
	if req.ServiceMethod != "SimpleEchoService.Echo" {
		return nil, fmt.Errorf("unknown service method: %s", req.ServiceMethod)
	}

	// Parse the arguments
	var args EchoArgs
	if err := json.Unmarshal(req.Args, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Create a reply object
	reply := &EchoReply{}

	// Call the service method
	ctx := context.Background()
	if err := service.Echo(ctx, &args, reply); err != nil {
		return nil, fmt.Errorf("service method execution failed: %w", err)
	}

	// Serialize the reply
	replyBytes, err := json.Marshal(reply)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize reply: %w", err)
	}

	// Create the response
	resp := SimpleJsonResponse{
		ServiceMethod: req.ServiceMethod,
		Sequence:      req.Sequence,
		Error:         "",
		Reply:         replyBytes,
	}

	// Serialize the response
	return json.Marshal(resp)
}
