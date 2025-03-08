package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"sync"

	"github.com/cloudwego/shmipc-go"
	"google.golang.org/protobuf/proto"
)

// ShmIPCServer implements the RPCServer interface using shmipc-go
type ShmIPCServer struct {
	endpoint    string
	services    map[string]*Service
	mutex       sync.RWMutex
	listener    net.Listener
	closing     bool
	closed      bool
	closeLock   sync.Mutex
	activeConns sync.WaitGroup
}

// NewShmIPCServer creates a new server with the shmipc-go transport
func NewShmIPCServer(endpoint string) *ShmIPCServer {
	return &ShmIPCServer{
		endpoint: endpoint,
		services: make(map[string]*Service),
	}
}

// Register registers a new service with the server
func (s *ShmIPCServer) Register(receiver interface{}) error {
	service, err := NewService(receiver)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, ok := s.services[service.Name]; ok {
		return fmt.Errorf("%w: %s", ErrServiceRegistered, service.Name)
	}

	s.services[service.Name] = service
	log.Printf("RPC: registered service %q", service.Name)
	return nil
}

// Start starts the RPC server
func (s *ShmIPCServer) Start() error {
	s.closeLock.Lock()
	if s.closing || s.closed {
		s.closeLock.Unlock()
		return errors.New("server is shutting down")
	}
	s.closeLock.Unlock()

	// Create and start the unix socket listener
	listener, err := shmipc.Listen(s.endpoint)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket %s: %w", s.endpoint, err)
	}
	s.listener = listener

	// Handle connections
	go s.acceptLoop()

	log.Printf("RPC server started on endpoint %s", s.endpoint)
	return nil
}

// acceptLoop accepts new connections and handles them
func (s *ShmIPCServer) acceptLoop() {
	for {
		s.closeLock.Lock()
		if s.closing || s.closed {
			s.closeLock.Unlock()
			return
		}
		s.closeLock.Unlock()

		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			s.closeLock.Lock()
			if !s.closing && !s.closed {
				// If the error is not due to shutdown, sleep and continue
				s.closeLock.Unlock()
				continue
			}
			s.closeLock.Unlock()
			return
		}

		s.activeConns.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single connection
func (s *ShmIPCServer) handleConnection(conn net.Conn) {
	defer s.activeConns.Done()
	defer conn.Close()

	// Create a shmipc session
	conf := shmipc.DefaultConfig()
	session, err := shmipc.Server(conn, conf)
	if err != nil {
		log.Printf("Failed to create shmipc session: %v", err)
		return
	}
	defer session.Close()

	// Accept a stream from the client
	stream, err := session.AcceptStream()
	if err != nil {
		log.Printf("Failed to accept stream: %v", err)
		return
	}
	defer stream.Close()

	// Read loop
	buffer := make([]byte, 4096)
	for {
		// Read from the stream
		n, err := stream.Read(buffer)
		if err != nil {
			log.Printf("Error reading from stream: %v", err)
			return
		}

		if n > 0 {
			// Make a copy of the data and process it in a separate goroutine
			data := make([]byte, n)
			copy(data, buffer[:n])
			go s.handleRequest(stream, data)
		}
	}
}

// handleRequest handles a single request
func (s *ShmIPCServer) handleRequest(stream *shmipc.Stream, data []byte) {
	// Deserialize the RPC message
	msg, err := deserializeRPCMessage(data)
	if err != nil {
		log.Printf("Failed to deserialize RPC message: %v", err)
		return
	}

	// Check if it's a request message
	if msg.Type != MessageTypeRequest {
		log.Printf("Received non-request message type: %d", msg.Type)
		return
	}

	// Create a context for the request
	ctx := context.Background()

	// Deserialize the request
	req, err := DeserializeRequest(msg.Payload, ctx)
	if err != nil {
		log.Printf("Failed to deserialize request: %v", err)
		s.sendErrorResponse(stream, "", 0, fmt.Sprintf("failed to deserialize request: %v", err), msg.RequestID)
		return
	}

	// Parse the service and method name
	serviceName, methodName, err := parseServiceMethod(req.ServiceMethod)
	if err != nil {
		log.Printf("Invalid service method: %v", err)
		s.sendErrorResponse(stream, req.ServiceMethod, req.Sequence, err.Error(), msg.RequestID)
		return
	}

	// Find the service
	s.mutex.RLock()
	service, ok := s.services[serviceName]
	s.mutex.RUnlock()
	if !ok {
		err := fmt.Errorf("%w: %s", ErrServiceNotFound, serviceName)
		log.Printf("Service not found: %v", err)
		s.sendErrorResponse(stream, req.ServiceMethod, req.Sequence, err.Error(), msg.RequestID)
		return
	}

	// Find the method
	mtype, ok := service.Methods[methodName]
	if !ok {
		err := fmt.Errorf("%w: %s.%s", ErrMethodNotFound, serviceName, methodName)
		log.Printf("Method not found: %v", err)
		s.sendErrorResponse(stream, req.ServiceMethod, req.Sequence, err.Error(), msg.RequestID)
		return
	}

	// Create a new instance of the argument type
	argValue := reflect.New(mtype.ArgType.Elem())
	argInterface := argValue.Interface()
	argMessage, ok := argInterface.(proto.Message)
	if !ok {
		err := fmt.Errorf("argument does not implement proto.Message")
		log.Printf("Invalid argument type: %v", err)
		s.sendErrorResponse(stream, req.ServiceMethod, req.Sequence, err.Error(), msg.RequestID)
		return
	}

	// Deserialize the argument from the request
	if req.Args != nil {
		argBytes, err := proto.Marshal(req.Args)
		if err != nil {
			log.Printf("Failed to marshal args: %v", err)
			s.sendErrorResponse(stream, req.ServiceMethod, req.Sequence, fmt.Sprintf("failed to marshal args: %v", err), msg.RequestID)
			return
		}
		if err := proto.Unmarshal(argBytes, argMessage); err != nil {
			log.Printf("Failed to unmarshal args: %v", err)
			s.sendErrorResponse(stream, req.ServiceMethod, req.Sequence, fmt.Sprintf("failed to unmarshal args: %v", err), msg.RequestID)
			return
		}
	}

	// Create a new instance of the reply type
	replyValue := reflect.New(mtype.ReplyType.Elem())
	replyInterface := replyValue.Interface()
	replyMessage, ok := replyInterface.(proto.Message)
	if !ok {
		err := fmt.Errorf("reply does not implement proto.Message")
		log.Printf("Invalid reply type: %v", err)
		s.sendErrorResponse(stream, req.ServiceMethod, req.Sequence, err.Error(), msg.RequestID)
		return
	}

	// Call the method
	err = service.Call(req.Context, methodName, argMessage, replyMessage)
	if err != nil {
		log.Printf("Error calling method %s.%s: %v", serviceName, methodName, err)
		s.sendErrorResponse(stream, req.ServiceMethod, req.Sequence, err.Error(), msg.RequestID)
		return
	}

	// Send the response
	s.sendResponse(stream, req.ServiceMethod, req.Sequence, "", replyMessage, msg.RequestID)
}

// sendErrorResponse sends an error response
func (s *ShmIPCServer) sendErrorResponse(stream *shmipc.Stream, serviceMethod string, sequence uint64, errorMsg, requestID string) {
	s.sendResponse(stream, serviceMethod, sequence, errorMsg, nil, requestID)
}

// sendResponse sends a response
func (s *ShmIPCServer) sendResponse(stream *shmipc.Stream, serviceMethod string, sequence uint64, errorMsg string, reply proto.Message, requestID string) {
	// Create the response
	resp := &RPCResponse{
		ServiceMethod: serviceMethod,
		Sequence:      sequence,
		Error:         errorMsg,
		Reply:         reply,
	}

	// Serialize the response
	respBytes, err := SerializeResponse(resp)
	if err != nil {
		log.Printf("Failed to serialize response: %v", err)
		return
	}

	// Create the RPC message
	msg := &RPCMessage{
		Type:      MessageTypeResponse,
		RequestID: requestID,
		Payload:   respBytes,
	}

	// Serialize the entire message
	msgBytes, err := serializeRPCMessage(msg)
	if err != nil {
		log.Printf("Failed to serialize RPC message: %v", err)
		return
	}

	// Send the message
	_, err = stream.Write(msgBytes)
	if err != nil {
		log.Printf("Failed to send response: %v", err)
	}
}

// Stop stops the RPC server
func (s *ShmIPCServer) Stop() error {
	s.closeLock.Lock()
	if s.closing || s.closed {
		s.closeLock.Unlock()
		return errors.New("server already stopping or closed")
	}
	s.closing = true
	s.closeLock.Unlock()

	// Close the listener
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			log.Printf("Error closing listener: %v", err)
		}
	}

	// Wait for all active connections to complete
	s.activeConns.Wait()

	s.closeLock.Lock()
	s.closed = true
	s.closing = false
	s.closeLock.Unlock()

	log.Printf("RPC server stopped")
	return nil
}
