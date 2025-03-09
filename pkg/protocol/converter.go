package protocol

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// 定义错误类型
var (
	// ErrUnknownMessageType 表示未知的消息类型错误
	ErrUnknownMessageType = errors.New("unknown message type")

	// ErrInvalidFormat 表示无效的消息格式错误
	ErrInvalidFormat = errors.New("invalid message format")

	// ErrUnsupportedFormat 表示不支持的消息格式错误
	ErrUnsupportedFormat = errors.New("unsupported message format")
)

// ConnectionType 表示连接类型
// 用于标识消息的来源和目标连接协议
type ConnectionType string

const (
	// ConnectionTypeWebSocket 表示WebSocket连接
	ConnectionTypeWebSocket ConnectionType = "WebSocket"

	// ConnectionTypeQUIC 表示QUIC连接
	ConnectionTypeQUIC ConnectionType = "QUIC"

	// ConnectionTypeInternal 表示内部连接（服务间通信）
	ConnectionTypeInternal ConnectionType = "Internal"
)

// FormatType 表示消息格式类型
// 用于指定消息的序列化格式
type FormatType int

const (
	// FormatTypeBinary 表示二进制格式
	// 二进制格式通常用于高效的通信，减少消息大小
	FormatTypeBinary FormatType = iota

	// FormatTypeJSON 表示JSON格式
	// JSON格式便于调试和与Web客户端交互
	FormatTypeJSON

	// FormatTypeText 表示文本格式
	// 文本格式用于简单的文本通信
	FormatTypeText

	// FormatTypeProtobuf 表示Protocol Buffers格式
	// Protobuf格式用于高效率的序列化/反序列化
	FormatTypeProtobuf
)

// ProtocolConverter 定义协议转换器接口
// 负责在不同协议格式和内部消息格式之间进行转换
// 这使得系统可以支持多种协议和格式，同时内部处理保持一致
type ProtocolConverter interface {
	// ToInternal 将原始消息转换为内部消息格式
	// 参数:
	//   - connType: 连接类型，表示消息的来源协议
	//   - formatType: 格式类型，表示消息的序列化格式
	//   - rawData: 原始二进制数据
	// 返回:
	//   - *Message: 转换后的内部消息对象
	//   - error: 转换过程中的错误
	ToInternal(connType ConnectionType, formatType FormatType, rawData []byte) (*Message, error)

	// FromInternal 将内部消息格式转换为原始消息
	// 参数:
	//   - connType: 连接类型，表示消息的目标协议
	//   - formatType: 格式类型，表示消息的序列化格式
	//   - msg: 内部消息对象
	// 返回:
	//   - []byte: 转换后的原始二进制数据
	//   - error: 转换过程中的错误
	FromInternal(connType ConnectionType, formatType FormatType, msg *Message) ([]byte, error)

	// RegisterMessageType 注册消息类型处理器
	// 可以为特定的服务类型和消息类型注册自定义处理器
	// 参数:
	//   - serviceType: 服务类型
	//   - messageType: 消息类型
	//   - processor: 消息处理器实现
	RegisterMessageType(serviceType ServiceType, messageType MessageType, processor MessageProcessor)
}

// MessageProcessor 定义消息处理器接口
// 负责处理特定类型的消息，实现具体的业务逻辑
type MessageProcessor interface {
	// ProcessMessage 处理特定类型的消息
	// 参数:
	//   - msg: 要处理的消息
	// 返回:
	//   - *Message: 处理后的响应消息，如无需响应可返回nil
	//   - error: 处理过程中的错误
	ProcessMessage(msg *Message) (*Message, error)
}

// DefaultProtocolConverter 实现默认的协议转换器
// 支持WebSocket、QUIC和内部通信协议，以及多种序列化格式
type DefaultProtocolConverter struct {
	processorsMutex sync.RWMutex                // 保护处理器映射的互斥锁
	processors      map[uint32]MessageProcessor // 消息处理器映射表，键为(serviceType << 16) | messageType
}

// NewProtocolConverter 创建一个新的默认协议转换器实例
// 返回已初始化好的转换器，可立即使用
func NewProtocolConverter() *DefaultProtocolConverter {
	return &DefaultProtocolConverter{
		processors: make(map[uint32]MessageProcessor),
	}
}

// makeProcessorKey 生成处理器映射键
// 将服务类型和消息类型合并为一个32位整数键
// 参数:
//   - serviceType: 服务类型
//   - messageType: 消息类型
//
// 返回: 组合后的32位键值
func makeProcessorKey(serviceType ServiceType, messageType MessageType) uint32 {
	return (uint32(serviceType) << 16) | uint32(messageType)
}

// RegisterMessageType 注册消息类型处理器
// 实现ProtocolConverter接口的方法
// 参数:
//   - serviceType: 服务类型
//   - messageType: 消息类型
//   - processor: 消息处理器实现
func (c *DefaultProtocolConverter) RegisterMessageType(serviceType ServiceType, messageType MessageType, processor MessageProcessor) {
	key := makeProcessorKey(serviceType, messageType)

	c.processorsMutex.Lock()
	defer c.processorsMutex.Unlock()

	c.processors[key] = processor
}

// GetProcessor 获取指定服务类型和消息类型的处理器
// 参数:
//   - serviceType: 服务类型
//   - messageType: 消息类型
//
// 返回:
//   - MessageProcessor: 找到的处理器
//   - bool: 是否找到处理器
func (c *DefaultProtocolConverter) GetProcessor(serviceType ServiceType, messageType MessageType) (MessageProcessor, bool) {
	key := makeProcessorKey(serviceType, messageType)

	c.processorsMutex.RLock()
	defer c.processorsMutex.RUnlock()

	processor, ok := c.processors[key]
	return processor, ok
}

// ToInternal 将原始消息转换为内部消息格式
// 根据连接类型和格式类型选择适当的转换方法
// 实现ProtocolConverter接口的方法
// 参数:
//   - connType: 连接类型
//   - formatType: 格式类型
//   - rawData: 原始二进制数据
//
// 返回:
//   - *Message: 转换后的内部消息
//   - error: 转换错误
func (c *DefaultProtocolConverter) ToInternal(connType ConnectionType, formatType FormatType, rawData []byte) (*Message, error) {
	// 根据连接类型选择转换方法
	var msg *Message
	var err error

	switch connType {
	case ConnectionTypeWebSocket:
		// 转换WebSocket消息
		msg, err = c.convertWebSocketToInternal(formatType, rawData)
	case ConnectionTypeQUIC:
		// 转换QUIC消息
		msg, err = c.convertQUICToInternal(formatType, rawData)
	case ConnectionTypeInternal:
		// 内部消息格式，直接解码
		msg, err = Decode(rawData)
	default:
		// 不支持的连接类型
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, connType)
	}

	if err != nil {
		return nil, err
	}

	return msg, nil
}

// FromInternal 将内部消息格式转换为原始消息
// 根据连接类型和格式类型选择适当的转换方法
// 实现ProtocolConverter接口的方法
// 参数:
//   - connType: 连接类型
//   - formatType: 格式类型
//   - msg: 内部消息对象
//
// 返回:
//   - []byte: 转换后的原始二进制数据
//   - error: 转换错误
func (c *DefaultProtocolConverter) FromInternal(connType ConnectionType, formatType FormatType, msg *Message) ([]byte, error) {
	switch connType {
	case ConnectionTypeWebSocket:
		// 转换为WebSocket消息
		return c.convertInternalToWebSocket(formatType, msg)
	case ConnectionTypeQUIC:
		// 转换为QUIC消息
		return c.convertInternalToQUIC(formatType, msg)
	case ConnectionTypeInternal:
		// 直接编码为内部消息格式
		return msg.Encode()
	default:
		// 不支持的连接类型
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, connType)
	}
}

// convertWebSocketToInternal 将WebSocket消息转换为内部格式
// 支持多种格式的WebSocket消息转换
// 参数:
//   - formatType: 消息格式类型
//   - rawData: 原始消息数据
//
// 返回:
//   - *Message: 转换后的内部消息
//   - error: 转换过程中的错误
func (c *DefaultProtocolConverter) convertWebSocketToInternal(formatType FormatType, rawData []byte) (*Message, error) {
	switch formatType {
	case FormatTypeBinary:
		// 二进制格式处理
		// 二进制格式为：1字节消息标志 + 2字节服务类型 + 2字节消息类型 + 负载
		if len(rawData) < 5 {
			// 数据过短，格式无效
			return nil, ErrInvalidFormat
		}

		// 解析消息标志（第1字节）
		msgFlag := rawData[0]
		// 解析服务类型（第2-3字节）
		serviceType := ServiceType(binary.BigEndian.Uint16(rawData[1:3]))
		// 解析消息类型（第4-5字节）
		messageType := MessageType(binary.BigEndian.Uint16(rawData[3:5]))
		// 提取负载数据（从第6字节开始）
		payload := rawData[5:]

		// 创建内部消息对象
		msg := NewMessage(serviceType, messageType, payload)
		msg.Flags = msgFlag
		return msg, nil

	case FormatTypeJSON:
		// JSON格式处理
		// 解析JSON格式的消息
		var jsonMsg struct {
			ServiceType uint16          `json:"service_type"`
			MessageType uint16          `json:"message_type"`
			Flags       uint8           `json:"flags"`
			Payload     json.RawMessage `json:"payload"`
			SessionID   string          `json:"session_id,omitempty"`
		}

		// 解析JSON数据
		if err := json.Unmarshal(rawData, &jsonMsg); err != nil {
			return nil, err
		}

		// 处理消息负载
		var payload []byte
		if len(jsonMsg.Payload) > 0 {
			// 检查负载是否为JSON字符串
			var stringPayload string
			if err := json.Unmarshal(jsonMsg.Payload, &stringPayload); err == nil {
				// 是JSON字符串，直接使用
				payload = []byte(stringPayload)
			} else {
				// 使用原始JSON负载
				payload = jsonMsg.Payload
			}
		} else {
			// 空负载
			payload = []byte{}
		}

		// 创建内部消息对象
		msg := NewMessage(
			ServiceType(jsonMsg.ServiceType),
			MessageType(jsonMsg.MessageType),
			payload,
		)
		msg.Flags = jsonMsg.Flags
		msg.SessionID = jsonMsg.SessionID

		return msg, nil

	case FormatTypeProtobuf:
		// Protobuf格式处理
		// 解析Protocol Buffers格式的消息
		var pbMsg anypb.Any
		if err := proto.Unmarshal(rawData, &pbMsg); err != nil {
			return nil, err
		}

		// 从类型URL中提取服务类型和消息类型
		// 类型URL格式："type.googleapis.com/serviceName.MessageName"
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

		// 提取服务名和消息名
		serviceName := typeParts[0]
		messageName := typeParts[1]

		// 将服务名和消息名映射到数值类型
		// 使用字符串哈希算法生成一个16位整数
		serviceType := ServiceType(uint16(hashString(serviceName) % 65535))
		messageType := MessageType(uint16(hashString(messageName) % 65535))

		// 创建内部消息对象
		msg := NewMessage(
			serviceType,
			messageType,
			pbMsg.GetValue(),
		)

		return msg, nil

	default:
		// 不支持的格式类型
		return nil, ErrUnsupportedFormat
	}
}

// convertQUICToInternal 将QUIC消息转换为内部格式
// 支持多种格式的QUIC消息转换
// 参数:
//   - formatType: 消息格式类型
//   - rawData: 原始消息数据
//
// 返回:
//   - *Message: 转换后的内部消息
//   - error: 转换过程中的错误
func (c *DefaultProtocolConverter) convertQUICToInternal(formatType FormatType, rawData []byte) (*Message, error) {
	// QUIC消息转换实现
	switch formatType {
	case FormatTypeBinary:
		// 二进制格式处理
		// 二进制格式为：1字节标志 + 2字节服务类型 + 2字节消息类型 + 负载
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
			return nil, fmt.Errorf("%w: %v", ErrInvalidFormat, err)
		}

		// 处理负载数据
		var payload []byte
		if len(jsonMsg.Payload) > 0 {
			// 检查是否是JSON字符串
			var stringPayload string
			if err := json.Unmarshal(jsonMsg.Payload, &stringPayload); err == nil {
				// 是JSON字符串，直接使用
				payload = []byte(stringPayload)
			} else {
				// 使用原始负载
				payload = jsonMsg.Payload
			}
		} else {
			payload = []byte{}
		}

		msg := NewMessage(
			ServiceType(jsonMsg.ServiceType),
			MessageType(jsonMsg.MessageType),
			payload,
		)
		msg.Flags = jsonMsg.Flags
		msg.SessionID = jsonMsg.SessionID

		return msg, nil

	case FormatTypeProtobuf:
		// 解析Protobuf格式
		var pbMsg anypb.Any
		if err := proto.Unmarshal(rawData, &pbMsg); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidFormat, err)
		}

		// 从类型URL提取服务类型和消息类型
		// URL格式："type.googleapis.com/serviceName.MessageName"
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

		// 使用哈希映射服务名和消息名
		serviceType := ServiceType(uint16(hashString(serviceName) % 65535))
		messageType := MessageType(uint16(hashString(messageName) % 65535))

		msg := NewMessage(
			serviceType,
			messageType,
			pbMsg.GetValue(),
		)

		return msg, nil

	case FormatTypeText:
		// 解析文本格式 (格式: "ServiceType:MessageType:Base64EncodedPayload")
		parts := strings.Split(string(rawData), ":")
		if len(parts) != 3 {
			return nil, ErrInvalidFormat
		}

		// 解析服务类型
		serviceTypeVal, err := strconv.ParseUint(parts[0], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid service type: %v", ErrInvalidFormat, err)
		}

		// 解析消息类型
		messageTypeVal, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid message type: %v", ErrInvalidFormat, err)
		}

		// 解码负载
		payload, err := base64.StdEncoding.DecodeString(parts[2])
		if err != nil {
			return nil, fmt.Errorf("%w: invalid payload encoding: %v", ErrInvalidFormat, err)
		}

		msg := NewMessage(
			ServiceType(serviceTypeVal),
			MessageType(messageTypeVal),
			payload,
		)

		return msg, nil

	default:
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedFormat, formatType)
	}
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
			ServiceType uint16      `json:"service_type"`
			MessageType uint16      `json:"message_type"`
			Flags       uint8       `json:"flags"`
			Payload     interface{} `json:"payload"`
			SessionID   string      `json:"session_id,omitempty"`
		}{
			ServiceType: uint16(msg.ServiceType),
			MessageType: uint16(msg.MessageType),
			Flags:       msg.Flags,
			SessionID:   msg.SessionID,
		}

		// Convert payload based on its content
		// If the payload is a valid string, use it directly
		if len(msg.Payload) > 0 {
			// Try to unmarshal as string first
			var s string
			if err := json.Unmarshal(msg.Payload, &s); err == nil {
				jsonMsg.Payload = s
			} else {
				// Use raw bytes if not a valid string
				jsonMsg.Payload = string(msg.Payload)
			}
		} else {
			jsonMsg.Payload = ""
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
	switch formatType {
	case FormatTypeBinary:
		// 构建二进制格式：1字节标志 + 2字节服务类型 + 2字节消息类型 + 负载
		result := make([]byte, 5+len(msg.Payload))
		result[0] = msg.Flags
		binary.BigEndian.PutUint16(result[1:3], uint16(msg.ServiceType))
		binary.BigEndian.PutUint16(result[3:5], uint16(msg.MessageType))
		copy(result[5:], msg.Payload)
		return result, nil

	case FormatTypeJSON:
		// 构建JSON消息
		jsonMsg := struct {
			ServiceType uint16          `json:"service_type"`
			MessageType uint16          `json:"message_type"`
			Flags       uint8           `json:"flags"`
			Payload     json.RawMessage `json:"payload"`
			SessionID   string          `json:"session_id,omitempty"`
		}{
			ServiceType: uint16(msg.ServiceType),
			MessageType: uint16(msg.MessageType),
			Flags:       msg.Flags,
			Payload:     msg.Payload,
			SessionID:   msg.SessionID,
		}

		// 如果负载是有效的JSON，则将其解析为JSON格式
		if len(msg.Payload) > 0 && msg.Payload[0] == '{' && msg.Payload[len(msg.Payload)-1] == '}' {
			var jsonPayload interface{}
			if err := json.Unmarshal(msg.Payload, &jsonPayload); err == nil {
				jsonBytes, err := json.Marshal(jsonPayload)
				if err == nil {
					jsonMsg.Payload = jsonBytes
				}
			}
		}

		return json.Marshal(jsonMsg)

	case FormatTypeProtobuf:
		// 构建Protobuf消息
		typeName := fmt.Sprintf("type.googleapis.com/%s.%s",
			serviceTypeToName(msg.ServiceType),
			messageTypeToName(msg.MessageType))

		anyMsg := &anypb.Any{
			TypeUrl: typeName,
			Value:   msg.Payload,
		}

		return proto.Marshal(anyMsg)

	case FormatTypeText:
		// 构建文本格式
		payloadBase64 := base64.StdEncoding.EncodeToString(msg.Payload)
		text := fmt.Sprintf("%d:%d:%s",
			uint16(msg.ServiceType),
			uint16(msg.MessageType),
			payloadBase64)
		return []byte(text), nil

	default:
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedFormat, formatType)
	}
}

// hashString 计算字符串的哈希值
func hashString(s string) int {
	h := 0
	for i := 0; i < len(s); i++ {
		h = 31*h + int(s[i])
	}
	return h
}

// serviceTypeToName 将服务类型转换为名称
func serviceTypeToName(serviceType ServiceType) string {
	// 实际应用中应有映射表，这里简化处理
	switch serviceType {
	case 1:
		return "Gateway"
	case 2:
		return "Chat"
	case 3:
		return "Game"
	case 4:
		return "Auth"
	default:
		return fmt.Sprintf("Service%d", serviceType)
	}
}

// messageTypeToName 将消息类型转换为名称
func messageTypeToName(messageType MessageType) string {
	// 实际应用中应有映射表，这里简化处理
	switch messageType {
	case 1:
		return "Request"
	case 2:
		return "Response"
	case 3:
		return "Notification"
	case 4:
		return "Error"
	default:
		return fmt.Sprintf("Message%d", messageType)
	}
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
