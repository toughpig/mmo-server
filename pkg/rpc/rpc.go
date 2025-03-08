package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// Common errors
var (
	ErrServiceNotFound   = errors.New("service not found")
	ErrMethodNotFound    = errors.New("method not found")
	ErrInvalidArgument   = errors.New("invalid argument")
	ErrTimeout           = errors.New("rpc call timeout")
	ErrConnectionClosed  = errors.New("connection closed")
	ErrServiceRegistered = errors.New("service already registered")
)

// RPCRequest represents an RPC request
type RPCRequest struct {
	ServiceMethod string        // format: "Service.Method"
	Sequence      uint64        // sequence number chosen by client
	Args          proto.Message // arguments
	Context       context.Context
}

// RPCResponse represents an RPC response
type RPCResponse struct {
	ServiceMethod string        // echoes that of the Request
	Sequence      uint64        // echoes that of the request
	Error         string        // error, if any
	Reply         proto.Message // reply message
}

// ServiceMethod parses a service and method name from a combined string
// format: "Service.Method"
func parseServiceMethod(serviceMethod string) (string, string, error) {
	dot := 0
	for i := 0; i < len(serviceMethod); i++ {
		if serviceMethod[i] == '.' {
			dot = i
			break
		}
	}
	if dot == 0 {
		return "", "", fmt.Errorf("service/method request ill-formed: %s", serviceMethod)
	}
	return serviceMethod[:dot], serviceMethod[dot+1:], nil
}

// MethodType represents a method defined in a service
type MethodType struct {
	Method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
}

// Service represents a registered service
type Service struct {
	Name     string
	Receiver reflect.Value
	Type     reflect.Type
	Methods  map[string]*MethodType
}

// NewService creates a new service for the given receiver
func NewService(receiver interface{}) (*Service, error) {
	s := &Service{
		Type:    reflect.TypeOf(receiver),
		Methods: make(map[string]*MethodType),
	}
	s.Name = reflect.Indirect(reflect.ValueOf(receiver)).Type().Name()
	if s.Name == "" {
		return nil, fmt.Errorf("no service name for type %s", s.Type.String())
	}
	s.Receiver = reflect.ValueOf(receiver)

	// Register methods
	for m := 0; m < s.Type.NumMethod(); m++ {
		method := s.Type.Method(m)
		mtype := method.Type

		// Method must be exported and have the correct signature:
		// func (t *T) MethodName(ctx context.Context, args *ArgType, reply *ReplyType) error
		if method.PkgPath != "" || // Unexported
			mtype.NumIn() != 4 || // (receiver, context, args, reply)
			mtype.In(1).String() != "context.Context" ||
			!isExportedOrBuiltinType(mtype.In(2)) ||
			!isExportedOrBuiltinType(mtype.In(3)) ||
			mtype.NumOut() != 1 || // (error)
			mtype.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}

		argType := mtype.In(2)
		replyType := mtype.In(3)

		s.Methods[method.Name] = &MethodType{
			Method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}

		log.Printf("Registered method %s.%s", s.Name, method.Name)
	}

	return s, nil
}

// isExportedOrBuiltinType checks if a type is exported or a builtin type
func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() == "" || t.PkgPath() == "proto_define" // For protobuf message types
}

// Call invokes the registered method
func (s *Service) Call(ctx context.Context, methodName string, args proto.Message, reply proto.Message) error {
	mtype, ok := s.Methods[methodName]
	if !ok {
		return fmt.Errorf("%w: %s.%s", ErrMethodNotFound, s.Name, methodName)
	}

	// Convert args to the expected type if needed
	var inArgs reflect.Value
	if args != nil {
		inArgs = reflect.ValueOf(args)
	} else {
		// If nil, create a new instance of the expected type
		inArgs = reflect.New(mtype.ArgType.Elem())
	}

	// Create a new instance of the reply type if nil
	var inReply reflect.Value
	if reply != nil {
		inReply = reflect.ValueOf(reply)
	} else {
		inReply = reflect.New(mtype.ReplyType.Elem())
	}

	// Call the method
	function := mtype.Method.Func
	returnValues := function.Call([]reflect.Value{
		s.Receiver,
		reflect.ValueOf(ctx),
		inArgs,
		inReply,
	})

	// Handle the error, if any
	errInter := returnValues[0].Interface()
	if errInter != nil {
		return errInter.(error)
	}

	return nil
}

// RPCClient defines the interface for an RPC client
type RPCClient interface {
	Call(ctx context.Context, serviceMethod string, args proto.Message, reply proto.Message) error
	Close() error
}

// RPCServer defines the interface for an RPC server
type RPCServer interface {
	Register(receiver interface{}) error
	Start() error
	Stop() error
}

// Generate a unique request ID
func generateRequestID() string {
	return uuid.New().String()
}

// RPC message types
const (
	MessageTypeRequest  = 1
	MessageTypeResponse = 2
)

// RPCMessage is the container for all RPC messages
type RPCMessage struct {
	Type      int    // 1=request, 2=response
	RequestID string // unique request ID
	Payload   []byte // serialized RPCRequest or RPCResponse
}

// SerializeRequest serializes an RPCRequest to bytes
func SerializeRequest(req *RPCRequest) ([]byte, error) {
	// Convert Args to protobuf bytes
	var argsBytes []byte
	var err error
	if req.Args != nil {
		argsBytes, err = proto.Marshal(req.Args)
		if err != nil {
			return nil, err
		}
	}

	// Create protocol request
	protocolReq := &ProtocolRPCRequest{
		ServiceMethod: req.ServiceMethod,
		Sequence:      req.Sequence,
		ArgsData:      argsBytes,
	}

	return protocolReq.MarshalBinary()
}

// DeserializeRequest deserializes bytes to an RPCRequest
func DeserializeRequest(data []byte, ctx context.Context) (*RPCRequest, error) {
	protocolReq := &ProtocolRPCRequest{}
	if err := protocolReq.UnmarshalBinary(data); err != nil {
		return nil, err
	}

	return &RPCRequest{
		ServiceMethod: protocolReq.ServiceMethod,
		Sequence:      protocolReq.Sequence,
		Args:          nil, // Will be created by service call
		Context:       ctx,
	}, nil
}

// SerializeResponse serializes an RPCResponse to bytes
func SerializeResponse(resp *RPCResponse) ([]byte, error) {
	// Convert Reply to protobuf bytes if not nil
	var replyBytes []byte
	var err error
	if resp.Reply != nil {
		replyBytes, err = proto.Marshal(resp.Reply)
		if err != nil {
			return nil, err
		}
	}

	// Create protocol response
	protocolResp := &ProtocolRPCResponse{
		ServiceMethod: resp.ServiceMethod,
		Sequence:      resp.Sequence,
		Error:         resp.Error,
		ReplyData:     replyBytes,
	}

	return protocolResp.MarshalBinary()
}

// DeserializeResponse deserializes bytes to an RPCResponse
func DeserializeResponse(data []byte) (*RPCResponse, error) {
	protocolResp := &ProtocolRPCResponse{}
	if err := protocolResp.UnmarshalBinary(data); err != nil {
		return nil, err
	}

	return &RPCResponse{
		ServiceMethod: protocolResp.ServiceMethod,
		Sequence:      protocolResp.Sequence,
		Error:         protocolResp.Error,
		Reply:         nil, // Will be created by client
	}, nil
}
