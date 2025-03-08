package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// SimpleClient is a basic RPC client for testing
type SimpleClient struct {
	conn         net.Conn
	socketPath   string
	seq          uint64
	mutex        sync.Mutex
	pending      map[uint64]*SimpleCall
	pendingMutex sync.Mutex
}

// SimpleCall represents an active RPC call
type SimpleCall struct {
	ServiceMethod string
	Sequence      uint64
	Response      chan *SimpleResponse
	Error         error
	Done          chan struct{}
}

// SimpleResponse represents an RPC response
type SimpleResponse struct {
	ServiceMethod string
	Sequence      uint64
	Error         string
	Response      []byte
}

// SimpleJsonRequest is a JSON-friendly request format
type SimpleJsonRequest struct {
	ServiceMethod string          `json:"service_method"`
	Sequence      uint64          `json:"sequence"`
	Args          json.RawMessage `json:"args"`
}

// SimpleJsonResponse is a JSON-friendly response format
type SimpleJsonResponse struct {
	ServiceMethod string          `json:"service_method"`
	Sequence      uint64          `json:"sequence"`
	Error         string          `json:"error"`
	Reply         json.RawMessage `json:"reply"`
}

// EchoArgs represents the arguments for the Echo method
type EchoArgs struct {
	Message string `json:"message"`
}

// EchoReply represents the reply from the Echo method
type EchoReply struct {
	Message string `json:"message"`
	Time    int64  `json:"time"`
}

// NewSimpleClient creates a new basic RPC client
func NewSimpleClient(socketPath string) (*SimpleClient, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", socketPath, err)
	}

	client := &SimpleClient{
		conn:       conn,
		socketPath: socketPath,
		pending:    make(map[uint64]*SimpleCall),
	}

	// Start response reader
	go client.readResponses()

	return client, nil
}

// Close closes the client connection
func (c *SimpleClient) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// readResponses reads responses from the server
func (c *SimpleClient) readResponses() {
	buffer := make([]byte, 4096)
	for {
		c.mutex.Lock()
		if c.conn == nil {
			c.mutex.Unlock()
			return
		}
		conn := c.conn
		c.mutex.Unlock()

		n, err := conn.Read(buffer)
		if err != nil {
			log.Printf("Error reading from connection: %v", err)
			c.mutex.Lock()
			c.conn = nil
			c.mutex.Unlock()
			return
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buffer[:n])
			go c.processResponse(data)
		}
	}
}

// processResponse processes a response from the server
func (c *SimpleClient) processResponse(data []byte) {
	// Deserialize the JSON response
	var resp SimpleJsonResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("Error deserializing response: %v", err)
		return
	}

	// Find the pending call
	c.pendingMutex.Lock()
	call, ok := c.pending[resp.Sequence]
	if ok {
		delete(c.pending, resp.Sequence)
	}
	c.pendingMutex.Unlock()

	if !ok {
		log.Printf("Response for unknown sequence: %d", resp.Sequence)
		return
	}

	// Send the response to the waiting call
	simpleResp := &SimpleResponse{
		ServiceMethod: resp.ServiceMethod,
		Sequence:      resp.Sequence,
		Error:         resp.Error,
		Response:      resp.Reply,
	}

	select {
	case call.Response <- simpleResp:
		// Sent successfully
	default:
		log.Printf("Failed to send response to call - channel full or closed")
	}

	close(call.Done)
}

// Call makes an RPC call to the server
func (c *SimpleClient) Call(ctx context.Context, serviceMethod string, message string) (*SimpleResponse, error) {
	// Create a new call
	seq := atomic.AddUint64(&c.seq, 1)
	call := &SimpleCall{
		ServiceMethod: serviceMethod,
		Sequence:      seq,
		Response:      make(chan *SimpleResponse, 1),
		Done:          make(chan struct{}),
	}

	// Register the call
	c.pendingMutex.Lock()
	c.pending[seq] = call
	c.pendingMutex.Unlock()

	// Create the request args
	args := EchoArgs{
		Message: message,
	}

	// Serialize the arguments
	argsBytes, err := json.Marshal(args)
	if err != nil {
		c.pendingMutex.Lock()
		delete(c.pending, seq)
		c.pendingMutex.Unlock()
		return nil, fmt.Errorf("error serializing arguments: %w", err)
	}

	// Create the request
	req := SimpleJsonRequest{
		ServiceMethod: serviceMethod,
		Sequence:      seq,
		Args:          argsBytes,
	}

	// Serialize the request
	reqBytes, err := json.Marshal(req)
	if err != nil {
		c.pendingMutex.Lock()
		delete(c.pending, seq)
		c.pendingMutex.Unlock()
		return nil, fmt.Errorf("error serializing request: %w", err)
	}

	// Send the message
	c.mutex.Lock()
	if c.conn == nil {
		c.mutex.Unlock()
		c.pendingMutex.Lock()
		delete(c.pending, seq)
		c.pendingMutex.Unlock()
		return nil, fmt.Errorf("connection closed")
	}
	conn := c.conn
	c.mutex.Unlock()

	_, err = conn.Write(reqBytes)
	if err != nil {
		c.pendingMutex.Lock()
		delete(c.pending, seq)
		c.pendingMutex.Unlock()
		return nil, fmt.Errorf("error sending message: %w", err)
	}

	// Wait for the response with a timeout
	select {
	case <-ctx.Done():
		c.pendingMutex.Lock()
		delete(c.pending, seq)
		c.pendingMutex.Unlock()
		return nil, ctx.Err()
	case resp := <-call.Response:
		if resp.Error != "" {
			return nil, fmt.Errorf("RPC error: %s", resp.Error)
		}
		return resp, nil
	case <-time.After(10 * time.Second):
		c.pendingMutex.Lock()
		delete(c.pending, seq)
		c.pendingMutex.Unlock()
		return nil, fmt.Errorf("call timed out")
	}
}

// CallEcho is a convenience method for making echo calls
func (c *SimpleClient) CallEcho(ctx context.Context, message string) (string, int64, error) {
	resp, err := c.Call(ctx, "SimpleEchoService.Echo", message)
	if err != nil {
		return "", 0, err
	}

	// Parse the response
	var reply EchoReply
	if err := json.Unmarshal(resp.Response, &reply); err != nil {
		return "", 0, fmt.Errorf("error deserializing response: %w", err)
	}

	return reply.Message, reply.Time, nil
}
