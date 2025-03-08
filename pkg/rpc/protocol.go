package rpc

import (
	"encoding/binary"
	"errors"
)

// ProtocolRPCRequest defines the wire format for RPC requests
type ProtocolRPCRequest struct {
	ServiceMethod string // Service.Method format
	Sequence      uint64 // Sequence number for matching requests with responses
	ArgsData      []byte // Serialized argument data
}

// ProtocolRPCResponse defines the wire format for RPC responses
type ProtocolRPCResponse struct {
	ServiceMethod string // Service.Method format (echoed from request)
	Sequence      uint64 // Sequence number (echoed from request)
	Error         string // Error message, empty if no error
	ReplyData     []byte // Serialized reply data
}

// MarshalBinary implements the encoding.BinaryMarshaler interface
func (r *ProtocolRPCRequest) MarshalBinary() ([]byte, error) {
	// Calculate total size
	serviceMethodLen := len(r.ServiceMethod)
	argsDataLen := len(r.ArgsData)
	totalLen := 8 + 4 + serviceMethodLen + 4 + argsDataLen // 8 bytes for sequence, 4 for service method length, service method bytes, 4 for args length, args bytes

	// Create buffer
	result := make([]byte, totalLen)
	offset := 0

	// Write sequence
	binary.BigEndian.PutUint64(result[offset:], r.Sequence)
	offset += 8

	// Write service method length and data
	binary.BigEndian.PutUint32(result[offset:], uint32(serviceMethodLen))
	offset += 4
	copy(result[offset:], r.ServiceMethod)
	offset += serviceMethodLen

	// Write args data length and data
	binary.BigEndian.PutUint32(result[offset:], uint32(argsDataLen))
	offset += 4
	copy(result[offset:], r.ArgsData)

	return result, nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface
func (r *ProtocolRPCRequest) UnmarshalBinary(data []byte) error {
	if len(data) < 16 { // Minimum size: 8 (sequence) + 4 (service method len) + 0 (service method) + 4 (args len) + 0 (args)
		return errors.New("invalid RPC request format: too short")
	}

	offset := 0

	// Read sequence
	r.Sequence = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Read service method
	serviceMethodLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if len(data) < int(offset+int(serviceMethodLen)) {
		return errors.New("invalid RPC request format: service method length exceeds data")
	}
	r.ServiceMethod = string(data[offset : offset+int(serviceMethodLen)])
	offset += int(serviceMethodLen)

	// Read args data
	if len(data) < offset+4 {
		return errors.New("invalid RPC request format: no args length")
	}
	argsDataLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if len(data) < int(offset+int(argsDataLen)) {
		return errors.New("invalid RPC request format: args length exceeds data")
	}
	r.ArgsData = make([]byte, argsDataLen)
	copy(r.ArgsData, data[offset:offset+int(argsDataLen)])

	return nil
}

// MarshalBinary implements the encoding.BinaryMarshaler interface
func (r *ProtocolRPCResponse) MarshalBinary() ([]byte, error) {
	// Calculate total size
	serviceMethodLen := len(r.ServiceMethod)
	errorLen := len(r.Error)
	replyDataLen := len(r.ReplyData)
	totalLen := 8 + 4 + serviceMethodLen + 4 + errorLen + 4 + replyDataLen // 8 bytes for sequence, 4 for service method length, service method bytes, 4 for error length, error bytes, 4 for reply length, reply bytes

	// Create buffer
	result := make([]byte, totalLen)
	offset := 0

	// Write sequence
	binary.BigEndian.PutUint64(result[offset:], r.Sequence)
	offset += 8

	// Write service method length and data
	binary.BigEndian.PutUint32(result[offset:], uint32(serviceMethodLen))
	offset += 4
	copy(result[offset:], r.ServiceMethod)
	offset += serviceMethodLen

	// Write error length and data
	binary.BigEndian.PutUint32(result[offset:], uint32(errorLen))
	offset += 4
	copy(result[offset:], r.Error)
	offset += errorLen

	// Write reply data length and data
	binary.BigEndian.PutUint32(result[offset:], uint32(replyDataLen))
	offset += 4
	copy(result[offset:], r.ReplyData)

	return result, nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface
func (r *ProtocolRPCResponse) UnmarshalBinary(data []byte) error {
	if len(data) < 20 { // Minimum size: 8 (sequence) + 4 (service method len) + 0 (service method) + 4 (error len) + 0 (error) + 4 (reply len) + 0 (reply)
		return errors.New("invalid RPC response format: too short")
	}

	offset := 0

	// Read sequence
	r.Sequence = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Read service method
	serviceMethodLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if len(data) < int(offset+int(serviceMethodLen)) {
		return errors.New("invalid RPC response format: service method length exceeds data")
	}
	r.ServiceMethod = string(data[offset : offset+int(serviceMethodLen)])
	offset += int(serviceMethodLen)

	// Read error
	if len(data) < offset+4 {
		return errors.New("invalid RPC response format: no error length")
	}
	errorLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if len(data) < int(offset+int(errorLen)) {
		return errors.New("invalid RPC response format: error length exceeds data")
	}
	r.Error = string(data[offset : offset+int(errorLen)])
	offset += int(errorLen)

	// Read reply data
	if len(data) < offset+4 {
		return errors.New("invalid RPC response format: no reply length")
	}
	replyDataLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if len(data) < int(offset+int(replyDataLen)) {
		return errors.New("invalid RPC response format: reply length exceeds data")
	}
	r.ReplyData = make([]byte, replyDataLen)
	copy(r.ReplyData, data[offset:offset+int(replyDataLen)])

	return nil
}
