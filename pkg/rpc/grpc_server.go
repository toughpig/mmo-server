package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"sync"
	"time"

	pb "mmo-server/proto_define"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// GRPCServer implements the RPCServer interface using gRPC
type GRPCServer struct {
	endpoint   string
	services   map[string]*Service
	mutex      sync.RWMutex
	grpcServer *grpc.Server
	closing    bool
	closed     bool
	closeLock  sync.Mutex
}

// RPCServiceServer implements the gRPC RPCService interface
type RPCServiceServer struct {
	pb.UnimplementedRPCServiceServer
	server *GRPCServer
}

// PlayerServiceServer implements the gRPC PlayerService interface
type PlayerServiceServer struct {
	pb.UnimplementedPlayerServiceServer
	server *GRPCServer
}

// MessageServiceServer implements the gRPC MessageService interface
type MessageServiceServer struct {
	pb.UnimplementedMessageServiceServer
	server *GRPCServer
}

// NewGRPCServer creates a new server with gRPC transport
func NewGRPCServer(endpoint string) *GRPCServer {
	return &GRPCServer{
		endpoint: endpoint,
		services: make(map[string]*Service),
	}
}

// Register registers a new service with the server
func (s *GRPCServer) Register(receiver interface{}) error {
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
func (s *GRPCServer) Start() error {
	s.closeLock.Lock()
	if s.closing || s.closed {
		s.closeLock.Unlock()
		return errors.New("server is shutting down")
	}
	s.closeLock.Unlock()

	// Create a TCP listener
	lis, err := net.Listen("tcp", s.endpoint)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.endpoint, err)
	}

	// Create a gRPC server with options for better performance and error handling
	serverOptions := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			// 记录请求和响应时间
			func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
				start := time.Now()
				resp, err := handler(ctx, req)
				log.Printf("gRPC call %s took %v", info.FullMethod, time.Since(start))
				return resp, err
			},
			// 错误恢复
			func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Recovered from panic in gRPC handler %s: %v", info.FullMethod, r)
					}
				}()
				return handler(ctx, req)
			},
		),
	}

	s.grpcServer = grpc.NewServer(serverOptions...)

	// Register the RPC service
	rpcService := &RPCServiceServer{server: s}
	pb.RegisterRPCServiceServer(s.grpcServer, rpcService)

	// Register the PlayerService if it exists
	s.mutex.RLock()
	if _, ok := s.services["PlayerService"]; ok {
		playerService := &PlayerServiceServer{server: s}
		pb.RegisterPlayerServiceServer(s.grpcServer, playerService)
		log.Printf("Registered direct gRPC service: PlayerService")
	}

	// Register the MessageService if it exists
	if _, ok := s.services["MessageService"]; ok {
		messageService := &MessageServiceServer{server: s}
		pb.RegisterMessageServiceServer(s.grpcServer, messageService)
		log.Printf("Registered direct gRPC service: MessageService")
	}
	s.mutex.RUnlock()

	// Start the gRPC server in a goroutine
	go func() {
		log.Printf("RPC server started on endpoint %s", s.endpoint)
		if err := s.grpcServer.Serve(lis); err != nil {
			if !s.closing && !s.closed {
				log.Printf("gRPC server error: %v", err)
			}
		}
	}()

	return nil
}

// Stop stops the RPC server
func (s *GRPCServer) Stop() error {
	s.closeLock.Lock()
	defer s.closeLock.Unlock()

	if s.closed {
		return nil
	}

	s.closing = true

	// Stop the gRPC server
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	s.closed = true
	s.closing = false

	log.Printf("RPC server stopped")
	return nil
}

// Call implements the RPCService.Call method
func (s *RPCServiceServer) Call(ctx context.Context, req *pb.RPCRequest) (*pb.RPCResponse, error) {
	// Parse the service and method name
	serviceName, methodName, err := parseServiceMethod(req.ServiceMethod)
	if err != nil {
		return &pb.RPCResponse{
			ServiceMethod: req.ServiceMethod,
			Sequence:      req.Sequence,
			Error:         err.Error(),
		}, nil
	}

	// Find the service
	s.server.mutex.RLock()
	service, ok := s.server.services[serviceName]
	s.server.mutex.RUnlock()
	if !ok {
		err := fmt.Errorf("%w: %s", ErrServiceNotFound, serviceName)
		return &pb.RPCResponse{
			ServiceMethod: req.ServiceMethod,
			Sequence:      req.Sequence,
			Error:         err.Error(),
		}, nil
	}

	// Find the method
	mtype, ok := service.Methods[methodName]
	if !ok {
		err := fmt.Errorf("%w: %s.%s", ErrMethodNotFound, serviceName, methodName)
		return &pb.RPCResponse{
			ServiceMethod: req.ServiceMethod,
			Sequence:      req.Sequence,
			Error:         err.Error(),
		}, nil
	}

	// Create a new instance of the argument type
	argValue := reflect.New(mtype.ArgType.Elem())
	argInterface := argValue.Interface()
	argMessage, ok := argInterface.(proto.Message)
	if !ok {
		err := fmt.Errorf("argument does not implement proto.Message")
		return &pb.RPCResponse{
			ServiceMethod: req.ServiceMethod,
			Sequence:      req.Sequence,
			Error:         err.Error(),
		}, nil
	}

	// Unpack the argument from the request
	if req.Args != nil {
		err := req.Args.UnmarshalTo(argMessage)
		if err != nil {
			return &pb.RPCResponse{
				ServiceMethod: req.ServiceMethod,
				Sequence:      req.Sequence,
				Error:         fmt.Sprintf("failed to unpack args: %v", err),
			}, nil
		}
	}

	// Create a new instance of the reply type
	replyValue := reflect.New(mtype.ReplyType.Elem())
	replyInterface := replyValue.Interface()
	replyMessage, ok := replyInterface.(proto.Message)
	if !ok {
		err := fmt.Errorf("reply does not implement proto.Message")
		return &pb.RPCResponse{
			ServiceMethod: req.ServiceMethod,
			Sequence:      req.Sequence,
			Error:         err.Error(),
		}, nil
	}

	// Call the method
	err = service.Call(ctx, methodName, argMessage, replyMessage)
	if err != nil {
		return &pb.RPCResponse{
			ServiceMethod: req.ServiceMethod,
			Sequence:      req.Sequence,
			Error:         err.Error(),
		}, nil
	}

	// Pack the reply into Any
	replyAny, err := anypb.New(replyMessage)
	if err != nil {
		return &pb.RPCResponse{
			ServiceMethod: req.ServiceMethod,
			Sequence:      req.Sequence,
			Error:         fmt.Sprintf("failed to pack reply: %v", err),
		}, nil
	}

	// Return the response
	return &pb.RPCResponse{
		ServiceMethod: req.ServiceMethod,
		Sequence:      req.Sequence,
		Reply:         replyAny,
	}, nil
}

// StreamCall implements the RPCService.StreamCall method
func (s *RPCServiceServer) StreamCall(stream pb.RPCService_StreamCallServer) error {
	for {
		// Receive a request
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		// Process the request
		resp, err := s.Call(stream.Context(), req)
		if err != nil {
			return err
		}

		// Send the response
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// UpdatePosition implements the PlayerService.UpdatePosition method
func (s *PlayerServiceServer) UpdatePosition(ctx context.Context, req *pb.PlayerPositionRequest) (*pb.PlayerPositionResponse, error) {
	log.Printf("PlayerService.UpdatePosition called for player: %s", req.PlayerId)

	// Find the service
	s.server.mutex.RLock()
	service, ok := s.server.services["PlayerService"]
	s.server.mutex.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, "PlayerService")
	}

	// Create a response object
	resp := &pb.PlayerPositionResponse{}

	// Call the method
	err := service.Call(ctx, "UpdatePosition", req, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// SendChat implements the MessageService.SendChat method
func (s *MessageServiceServer) SendChat(ctx context.Context, req *pb.ChatRequest) (*pb.ChatResponse, error) {
	// Find the service
	s.server.mutex.RLock()
	service, ok := s.server.services["MessageService"]
	s.server.mutex.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, "MessageService")
	}

	// Create a response object
	resp := &pb.ChatResponse{}

	// Call the method
	err := service.Call(ctx, "SendChat", req, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// ReceiveEvents implements the MessageService.ReceiveEvents method
func (s *MessageServiceServer) ReceiveEvents(req *pb.HeartbeatRequest, stream pb.MessageService_ReceiveEventsServer) error {
	// Find the service
	s.server.mutex.RLock()
	_, ok := s.server.services["MessageService"]
	s.server.mutex.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, "MessageService")
	}

	// Create a channel for events
	eventChan := make(chan *pb.PositionSyncResponse, 100)

	// Start a goroutine to handle events
	go func() {
		for event := range eventChan {
			if err := stream.Send(event); err != nil {
				log.Printf("Error sending event: %v", err)
				return
			}
		}
	}()

	// This is a placeholder - in a real implementation, we would register the handler
	// with the service and wait for events
	log.Printf("ReceiveEvents called, but not fully implemented")

	// Block until the client disconnects
	<-stream.Context().Done()
	close(eventChan)

	return nil
}
