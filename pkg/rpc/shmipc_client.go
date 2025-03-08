package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/shmipc-go"
	"google.golang.org/protobuf/proto"
)

// ShmIPCClient implements the RPCClient interface using shmipc-go
type ShmIPCClient struct {
	endpoint    string
	conn        net.Conn
	session     *shmipc.Session
	stream      *shmipc.Stream
	seq         uint64
	pending     map[uint64]*Call
	pendingLock sync.Mutex
	closing     bool
	closed      bool
	mutex       sync.Mutex
}

// Call represents an active RPC call
type Call struct {
	ServiceMethod string        // The name of the service and method to call
	Args          proto.Message // The argument to the function
	Reply         proto.Message // The reply from the function
	Error         error         // After completion, the error status
	Done          chan *Call    // Signaled when call is complete
	ctx           context.Context
	cancelFunc    context.CancelFunc
}

// NewShmIPCClient creates a new client with the shmipc-go transport
func NewShmIPCClient(endpoint string) (*ShmIPCClient, error) {
	// Connect to the server
	conn, err := net.Dial("unix", endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to dial unix socket %s: %w", endpoint, err)
	}

	// Create a shmipc session
	conf := shmipc.DefaultConfig()
	session, err := shmipc.Server(conn, conf)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create shmipc session: %w", err)
	}

	// Open a stream for communication
	stream, err := session.OpenStream()
	if err != nil {
		session.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	shmClient := &ShmIPCClient{
		endpoint: endpoint,
		conn:     conn,
		session:  session,
		stream:   stream,
		pending:  make(map[uint64]*Call),
	}

	go shmClient.receive()

	return shmClient, nil
}

// receive runs in a separate goroutine, receiving messages from the server
func (c *ShmIPCClient) receive() {
	buffer := make([]byte, 4096)
	for {
		// Check if client is closed
		c.mutex.Lock()
		if c.closing || c.closed {
			c.mutex.Unlock()
			return
		}
		c.mutex.Unlock()

		// Read from the stream
		n, err := c.stream.Read(buffer)
		if err != nil {
			log.Printf("Error reading from shmipc: %v", err)

			c.mutex.Lock()
			if !c.closing && !c.closed {
				// Connection error, close the client
				c.Close()
			}
			c.mutex.Unlock()
			return
		}

		if n > 0 {
			// Make a copy of the data and process it in a separate goroutine
			data := make([]byte, n)
			copy(data, buffer[:n])
			go c.processResponse(data)
		}
	}
}

// nextSequence generates the next sequence number
func (c *ShmIPCClient) nextSequence() uint64 {
	return atomic.AddUint64(&c.seq, 1)
}

// Call performs a synchronous RPC call
func (c *ShmIPCClient) Call(ctx context.Context, serviceMethod string, args proto.Message, reply proto.Message) error {
	call := c.Go(ctx, serviceMethod, args, reply, make(chan *Call, 1))
	<-call.Done
	return call.Error
}

// Go starts an asynchronous RPC call
func (c *ShmIPCClient) Go(ctx context.Context, serviceMethod string, args proto.Message, reply proto.Message, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10) // buffered
	}

	// Check if client is closed
	c.mutex.Lock()
	if c.closing || c.closed {
		c.mutex.Unlock()
		call := &Call{
			ServiceMethod: serviceMethod,
			Args:          args,
			Reply:         reply,
			Error:         ErrConnectionClosed,
			Done:          done,
		}
		call.done()
		return call
	}
	c.mutex.Unlock()

	// Create a context with a timeout if the parent context doesn't have one
	callCtx := ctx
	var cancelFunc context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		// Default timeout of 60 seconds
		callCtx, cancelFunc = context.WithTimeout(ctx, 60*time.Second)
	}

	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
		ctx:           callCtx,
		cancelFunc:    cancelFunc,
	}

	sequence := c.nextSequence()
	c.sendRequest(call, sequence)
	return call
}

// sendRequest sends the request to the server
func (c *ShmIPCClient) sendRequest(call *Call, seq uint64) {
	// Register call in pending map
	c.pendingLock.Lock()
	c.pending[seq] = call
	c.pendingLock.Unlock()

	// Create and send request
	req := &RPCRequest{
		ServiceMethod: call.ServiceMethod,
		Sequence:      seq,
		Args:          call.Args,
		Context:       call.ctx,
	}

	// Serialize the request
	reqBytes, err := SerializeRequest(req)
	if err != nil {
		c.removeCall(seq)
		call.Error = err
		call.done()
		return
	}

	// Create the RPC message
	msg := &RPCMessage{
		Type:      MessageTypeRequest,
		RequestID: generateRequestID(),
		Payload:   reqBytes,
	}

	// Serialize the entire message
	msgBytes, err := serializeRPCMessage(msg)
	if err != nil {
		c.removeCall(seq)
		call.Error = err
		call.done()
		return
	}

	// Send the message using the stream
	_, err = c.stream.Write(msgBytes)
	if err != nil {
		c.removeCall(seq)
		call.Error = err
		call.done()
		return
	}

	// Start a goroutine to handle timeouts
	go c.handleCallTimeout(call, seq)
}

// handleCallTimeout handles the timeout for a call
func (c *ShmIPCClient) handleCallTimeout(call *Call, seq uint64) {
	select {
	case <-call.ctx.Done():
		if call.ctx.Err() != nil && !errors.Is(call.ctx.Err(), context.Canceled) {
			// Remove the call and set the timeout error
			c.removeCall(seq)
			call.Error = ErrTimeout
			call.done()
		}
	case <-call.Done:
		// Call completed normally or was cancelled
		return
	}
}

// removeCall removes a call from the pending map
func (c *ShmIPCClient) removeCall(seq uint64) *Call {
	c.pendingLock.Lock()
	defer c.pendingLock.Unlock()

	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

// processResponse processes a response from the server
func (c *ShmIPCClient) processResponse(data []byte) {
	// Deserialize the RPC message
	msg, err := deserializeRPCMessage(data)
	if err != nil {
		log.Printf("Failed to deserialize RPC message: %v", err)
		return
	}

	// Check if it's a response message
	if msg.Type != MessageTypeResponse {
		log.Printf("Received non-response message type: %d", msg.Type)
		return
	}

	// Deserialize the response
	resp, err := DeserializeResponse(msg.Payload)
	if err != nil {
		log.Printf("Failed to deserialize response: %v", err)
		return
	}

	// Get the call
	seq := resp.Sequence
	call := c.removeCall(seq)
	if call == nil {
		log.Printf("Unknown sequence for response: %d", seq)
		return
	}

	// If there was an error from the server, set it
	if resp.Error != "" {
		call.Error = errors.New(resp.Error)
		call.done()
		return
	}

	// If the response has a reply, set it on the call
	if resp.Reply != nil && call.Reply != nil {
		// Use reflection to copy the reply
		proto.Merge(call.Reply, resp.Reply)
	}

	call.done()
}

// done marks the call as complete
func (c *Call) done() {
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	select {
	case c.Done <- c:
		// ok
	default:
		// Channel is full or closed, which shouldn't happen
		log.Printf("RPC: discarding Call reply due to insufficient Done chan capacity")
	}
}

// Close closes the client connection
func (c *ShmIPCClient) Close() error {
	c.mutex.Lock()
	if c.closing || c.closed {
		c.mutex.Unlock()
		return ErrConnectionClosed
	}
	c.closing = true
	c.mutex.Unlock()

	// Complete all pending calls with an error
	c.pendingLock.Lock()
	for _, call := range c.pending {
		call.Error = ErrConnectionClosed
		call.done()
	}
	c.pending = nil
	c.pendingLock.Unlock()

	// Close the stream, session, and connection
	var err error
	if c.stream != nil {
		err = c.stream.Close()
	}
	if c.session != nil {
		c.session.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}

	c.mutex.Lock()
	c.closed = true
	c.closing = false
	c.mutex.Unlock()

	return err
}

// serializeRPCMessage serializes an RPCMessage to bytes
func serializeRPCMessage(msg *RPCMessage) ([]byte, error) {
	// Simple format: [4 bytes for type][16 bytes for requestID][remaining bytes for payload]
	typeBytes := make([]byte, 4)
	for i := 0; i < 4; i++ {
		typeBytes[i] = byte((msg.Type >> (8 * (3 - i))) & 0xFF)
	}

	requestIDBytes := []byte(msg.RequestID)
	if len(requestIDBytes) > 36 {
		requestIDBytes = requestIDBytes[:36]
	} else if len(requestIDBytes) < 36 {
		// Pad with zeros
		temp := make([]byte, 36)
		copy(temp, requestIDBytes)
		requestIDBytes = temp
	}

	result := make([]byte, 4+36+len(msg.Payload))
	copy(result[0:4], typeBytes)
	copy(result[4:40], requestIDBytes)
	copy(result[40:], msg.Payload)

	return result, nil
}

// deserializeRPCMessage deserializes bytes to an RPCMessage
func deserializeRPCMessage(data []byte) (*RPCMessage, error) {
	if len(data) < 40 {
		return nil, errors.New("invalid message format: too short")
	}

	typeBytes := data[0:4]
	msgType := 0
	for i := 0; i < 4; i++ {
		msgType = (msgType << 8) | int(typeBytes[i])
	}

	requestIDBytes := data[4:40]
	requestID := string(requestIDBytes)

	payload := data[40:]

	return &RPCMessage{
		Type:      msgType,
		RequestID: requestID,
		Payload:   payload,
	}, nil
}
