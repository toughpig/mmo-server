package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	ErrUnknownMessageType = errors.New("unknown message type")
	ErrInvalidFormat      = errors.New("invalid message format")
	ErrUnsupportedFormat  = errors.New("unsupported message format")
)

// ConnectionType 连接类型
type ConnectionType string

const (
	ConnectionTypeWebSocket ConnectionType = "WebSocket"
	ConnectionTypeQUIC      ConnectionType = "QUIC"
	ConnectionTypeInternal  ConnectionType = "Internal"
)

// FormatType 消息格式类型
type FormatType int

const (
	FormatTypeBinary   FormatType = iota // 二进制格式
	FormatTypeJSON                       // JSON格式
	FormatTypeText                       // 文本格式
	FormatTypeProtobuf                   // Protocol Buffers格式
)

// ProtocolConverter 协议转换器接口
type ProtocolConverter interface {
	// ToInternal 将原始消息转换为内部消息格式
	ToInternal(connType ConnectionType, formatType FormatType, rawData []byte) (*Message, error)

	// FromInternal 将内部消息格式转换为原始消息
	FromInternal(connType ConnectionType, formatType FormatType, msg *Message) ([]byte, error)

	// RegisterMessageType 注册消息类型处理器
	RegisterMessageType(serviceType ServiceType, messageType MessageType, processor MessageProcessor)
}

// MessageProcessor 消息处理器接口
type MessageProcessor interface {
	// ProcessMessage 处理特定类型的消息
	ProcessMessage(msg *Message) (*Message, error)
}

// DefaultProtocolConverter 默认协议转换器实现
type DefaultProtocolConverter struct {
	processorsMutex sync.RWMutex
	processors      map[uint32]MessageProcessor // key = (serviceType << 16) | messageType
}

// NewProtocolConverter 创建一个新的协议转换器
func NewProtocolConverter() *DefaultProtocolConverter {
	return &DefaultProtocolConverter{
		processors: make(map[uint32]MessageProcessor),
	}
}

// 生成处理器映射键
func makeProcessorKey(serviceType ServiceType, messageType MessageType) uint32 {
	return (uint32(serviceType) << 16) | uint32(messageType)
}

// RegisterMessageType 注册消息类型处理器
func (c *DefaultProtocolConverter) RegisterMessageType(serviceType ServiceType, messageType MessageType, processor MessageProcessor) {
	key := makeProcessorKey(serviceType, messageType)

	c.processorsMutex.Lock()
	defer c.processorsMutex.Unlock()

	c.processors[key] = processor
}

// GetProcessor 获取消息处理器
func (c *DefaultProtocolConverter) GetProcessor(serviceType ServiceType, messageType MessageType) (MessageProcessor, bool) {
	key := makeProcessorKey(serviceType, messageType)

	c.processorsMutex.RLock()
	defer c.processorsMutex.RUnlock()

	processor, ok := c.processors[key]
	return processor, ok
}

// ToInternal 将原始消息转换为内部消息格式
func (c *DefaultProtocolConverter) ToInternal(connType ConnectionType, formatType FormatType, rawData []byte) (*Message, error) {
	// 解析消息
	var msg *Message
	var err error

	switch connType {
	case ConnectionTypeWebSocket:
		msg, err = c.convertWebSocketToInternal(formatType, rawData)
	case ConnectionTypeQUIC:
		msg, err = c.convertQUICToInternal(formatType, rawData)
	case ConnectionTypeInternal:
		// 内部消息格式，直接解码
		msg, err = Decode(rawData)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, connType)
	}

	if err != nil {
		return nil, err
	}

	return msg, nil
}

// FromInternal 将内部消息格式转换为原始消息
func (c *DefaultProtocolConverter) FromInternal(connType ConnectionType, formatType FormatType, msg *Message) ([]byte, error) {
	switch connType {
	case ConnectionTypeWebSocket:
		return c.convertInternalToWebSocket(formatType, msg)
	case ConnectionTypeQUIC:
		return c.convertInternalToQUIC(formatType, msg)
	case ConnectionTypeInternal:
		// 直接编码为内部消息格式
		return msg.Encode()
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, connType)
	}
}

// convertWebSocketToInternal 将WebSocket消息转换为内部格式
func (c *DefaultProtocolConverter) convertWebSocketToInternal(formatType FormatType, rawData []byte) (*Message, error) {
	switch formatType {
	case FormatTypeBinary:
		// 假设二进制格式为：1字节消息类型 + 2字节服务类型 + 2字节消息类型 + 负载
		if len(rawData) < 5 {
			return nil, ErrInvalidFormat
		}

		msgFlag := rawData[0]
		serviceType := ServiceType(binary.BigEndian.Uint16(rawData[1:3]))
		messageType := MessageType(binary.BigEndian.Uint16(rawData[3:5]))
		payload := rawData[5:]

		msg := NewMessage(serviceType, messageType, payload)
		msg.Flags = msgFlag
		return msg, nil

	case FormatTypeJSON:
		// 解析JSON格式
		var jsonMsg struct {
			ServiceType uint16          `json:"service_type"`
			MessageType uint16          `json:"message_type"`
			Flags       uint8           `json:"flags"`
			Payload     json.RawMessage `json:"payload"`
			SessionID   string          `json:"session_id,omitempty"`
		}

		if err := json.Unmarshal(rawData, &jsonMsg); err != nil {
			return nil, err
		}

		msg := NewMessage(
			ServiceType(jsonMsg.ServiceType),
			MessageType(jsonMsg.MessageType),
			[]byte(jsonMsg.Payload),
		)
		msg.Flags = jsonMsg.Flags
		msg.SessionID = jsonMsg.SessionID

		return msg, nil

	case FormatTypeProtobuf:
		// 解析Protobuf格式
		var pbMsg anypb.Any
		if err := proto.Unmarshal(rawData, &pbMsg); err != nil {
			return nil, err
		}

		// 从类型URL中提取服务类型和消息类型
		// 假设URL格式为："type.googleapis.com/serviceName.MessageName"
		typeURL := pbMsg.GetTypeUrl()
		parts := strings.Split(typeURL, "/")
		if len(parts) != 2 {
			return nil, ErrInvalidFormat
		}

		typeName := parts[1]
		typeParts := strings.Split(typeName, ".")
		if len(typeParts) != 2 {
			return nil, ErrInvalidFormat
		}

		serviceName := typeParts[0]
		messageName := typeParts[1]

		// 这里需要一个映射来将服务名和消息名映射到服务类型和消息类型
		// 简化处理，这里使用字符串哈希
		serviceType := ServiceType(uint16(hashString(serviceName) % 65535))
		messageType := MessageType(uint16(hashString(messageName) % 65535))

		msg := NewMessage(
			serviceType,
			messageType,
			pbMsg.GetValue(),
		)

		return msg, nil

	default:
		return nil, ErrUnsupportedFormat
	}
}

// convertQUICToInternal 将QUIC消息转换为内部格式
func (c *DefaultProtocolConverter) convertQUICToInternal(formatType FormatType, rawData []byte) (*Message, error) {
	// QUIC转换逻辑与WebSocket类似
	return c.convertWebSocketToInternal(formatType, rawData)
}

// convertInternalToWebSocket 将内部格式转换为WebSocket消息
func (c *DefaultProtocolConverter) convertInternalToWebSocket(formatType FormatType, msg *Message) ([]byte, error) {
	switch formatType {
	case FormatTypeBinary:
		// 构建二进制格式：1字节消息类型 + 2字节服务类型 + 2字节消息类型 + 负载
		result := make([]byte, 5+len(msg.Payload))
		result[0] = msg.Flags
		binary.BigEndian.PutUint16(result[1:3], uint16(msg.ServiceType))
		binary.BigEndian.PutUint16(result[3:5], uint16(msg.MessageType))
		copy(result[5:], msg.Payload)
		return result, nil

	case FormatTypeJSON:
		// 构建JSON格式
		jsonMsg := struct {
			ServiceType uint16 `json:"service_type"`
			MessageType uint16 `json:"message_type"`
			Flags       uint8  `json:"flags"`
			Payload     []byte `json:"payload"`
			SessionID   string `json:"session_id,omitempty"`
		}{
			ServiceType: uint16(msg.ServiceType),
			MessageType: uint16(msg.MessageType),
			Flags:       msg.Flags,
			Payload:     msg.Payload,
			SessionID:   msg.SessionID,
		}

		return json.Marshal(jsonMsg)

	case FormatTypeProtobuf:
		// 构建Protobuf格式
		// 这里需要根据服务类型和消息类型构造类型URL
		// 简化处理，使用固定格式
		typeURL := fmt.Sprintf("type.googleapis.com/service%d.message%d", msg.ServiceType, msg.MessageType)

		pbMsg := &anypb.Any{
			TypeUrl: typeURL,
			Value:   msg.Payload,
		}

		return proto.Marshal(pbMsg)

	default:
		return nil, ErrUnsupportedFormat
	}
}

// convertInternalToQUIC 将内部格式转换为QUIC消息
func (c *DefaultProtocolConverter) convertInternalToQUIC(formatType FormatType, msg *Message) ([]byte, error) {
	// QUIC转换逻辑与WebSocket类似
	return c.convertInternalToWebSocket(formatType, msg)
}

// hashString 计算字符串的简单哈希值
func hashString(s string) uint32 {
	h := uint32(0)
	for i := 0; i < len(s); i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}

// MessageTypeInfo 消息类型信息
type MessageTypeInfo struct {
	ServiceType    ServiceType
	MessageType    MessageType
	ServiceName    string
	MessageName    string
	RequestType    reflect.Type
	ResponseType   reflect.Type
	IsRequest      bool
	CorrelationKey string
}

// MessageRegistry 消息类型注册表
type MessageRegistry struct {
	mu                sync.RWMutex
	typeInfoByID      map[uint32]*MessageTypeInfo
	typeInfoByName    map[string]*MessageTypeInfo
	typeInfoByReqType map[reflect.Type]*MessageTypeInfo
}

// NewMessageRegistry 创建消息类型注册表
func NewMessageRegistry() *MessageRegistry {
	return &MessageRegistry{
		typeInfoByID:      make(map[uint32]*MessageTypeInfo),
		typeInfoByName:    make(map[string]*MessageTypeInfo),
		typeInfoByReqType: make(map[reflect.Type]*MessageTypeInfo),
	}
}

// RegisterMessageType 注册消息类型
func (r *MessageRegistry) RegisterMessageType(info *MessageTypeInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := makeProcessorKey(info.ServiceType, info.MessageType)
	fullName := fmt.Sprintf("%s.%s", info.ServiceName, info.MessageName)

	r.typeInfoByID[key] = info
	r.typeInfoByName[fullName] = info

	if info.RequestType != nil {
		r.typeInfoByReqType[info.RequestType] = info
	}
}

// GetTypeInfoByID 通过ID获取类型信息
func (r *MessageRegistry) GetTypeInfoByID(serviceType ServiceType, messageType MessageType) (*MessageTypeInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := makeProcessorKey(serviceType, messageType)
	info, ok := r.typeInfoByID[key]
	return info, ok
}

// GetTypeInfoByName 通过名称获取类型信息
func (r *MessageRegistry) GetTypeInfoByName(fullName string) (*MessageTypeInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.typeInfoByName[fullName]
	return info, ok
}

// GetTypeInfoByRequestType 通过请求类型获取类型信息
func (r *MessageRegistry) GetTypeInfoByRequestType(reqType reflect.Type) (*MessageTypeInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.typeInfoByReqType[reqType]
	return info, ok
}
