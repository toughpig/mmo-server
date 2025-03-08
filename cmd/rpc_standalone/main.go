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
)

func main() {
	// Parse command-line flags
	mode := flag.String("mode", "server", "Mode to run in: 'server' or 'client'")
	socket := flag.String("socket", "/tmp/rpc-standalone.sock", "Path to Unix socket")
	message := flag.String("message", "Hello, RPC!", "Message to send (client mode only)")
	flag.Parse()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	switch *mode {
	case "server":
		// Run the server
		fmt.Printf("Starting RPC server on socket: %s\n", *socket)

		// Run the server (this will block until a signal is received)
		rpc.StartStandaloneServer(*socket)

	case "client":
		// Run the client
		fmt.Printf("Connecting to RPC server at socket: %s\n", *socket)

		// Create a client
		client, err := rpc.NewSimpleClient(*socket)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		// Create a context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Make the call
		fmt.Printf("Sending message: %s\n", *message)
		response, timestamp, err := client.CallEcho(ctx, *message)
		if err != nil {
			log.Fatalf("RPC call failed: %v", err)
		}

		// Print the response
		fmt.Printf("Response: %s (at timestamp %d)\n", response, timestamp)

	default:
		fmt.Printf("Unknown mode: %s\n", *mode)
		flag.Usage()
		os.Exit(1)
	}
}
