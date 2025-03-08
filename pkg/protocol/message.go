package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ServiceType 定义服务类型
type ServiceType uint16

// 服务类型常量
const (
	ServiceTypeUnknown    ServiceType = 0
	ServiceTypeGateway    ServiceType = 1
	ServiceTypeChat       ServiceType = 2
	ServiceTypeGame       ServiceType = 3
	ServiceTypeAuth       ServiceType = 4
	ServiceTypeMatchmaker ServiceType = 5
	ServiceTypeLocation   ServiceType = 6
	ServiceTypeCombat     ServiceType = 7
	ServiceTypeInventory  ServiceType = 8
	ServiceTypeQuest      ServiceType = 9
)

// MessageType 定义消息类型
type MessageType uint16

// 消息版本
const (
	ProtocolVersion = 1
)

// 消息标志位
const (
	FlagNone       uint8 = 0x00
	FlagEncrypted  uint8 = 0x01 // 消息已加密
	FlagCompressed uint8 = 0x02 // 消息已压缩
	FlagReliable   uint8 = 0x04 // 消息要求可靠传输
	FlagOrdered    uint8 = 0x08 // 消息要求有序传输
	FlagAck        uint8 = 0x10 // 消息是确认消息
	FlagSystem     uint8 = 0x20 // 系统消息
	FlagPing       uint8 = 0x40 // Ping消息
	FlagPong       uint8 = 0x80 // Pong消息
)

// 内部消息最大大小
const (
	MaxMessageSize     = 10 * 1024 * 1024 // 10MB
	HeaderSize         = 20               // 20字节消息头
	MaxPayloadSize     = MaxMessageSize - HeaderSize
	MaxServiceNameSize = 32
)

// Message 内部消息格式
type Message struct {
	// 消息头 (20字节固定)
	Version       uint8       // 1字节: 协议版本
	Flags         uint8       // 1字节: 消息标志位
	ServiceType   ServiceType // 2字节: 服务类型
	MessageType   MessageType // 2字节: 消息类型
	SequenceID    uint32      // 4字节: 序列号，用于有序传输和确认
	Timestamp     uint64      // 8字节: 消息创建时间戳(微秒)
	PayloadLength uint32      // 4字节: 负载长度

	// 可变长度字段
	SourceService      string // 发送者服务名
	DestinationService string // 目标服务名
	CorrelationID      string // 关联ID，用于追踪请求-响应
	SessionID          string // 会话ID

	// 消息负载
	Payload []byte // 实际消息数据
}

// NewMessage 创建新消息
func NewMessage(serviceType ServiceType, messageType MessageType, payload []byte) *Message {
	return &Message{
		Version:       ProtocolVersion,
		Flags:         FlagNone,
		ServiceType:   serviceType,
		MessageType:   messageType,
		SequenceID:    0, // 需要设置
		Timestamp:     uint64(time.Now().UnixMicro()),
		PayloadLength: uint32(len(payload)),
		Payload:       payload,
	}
}

// SetFlag 设置标志位
func (m *Message) SetFlag(flag uint8) {
	m.Flags |= flag
}

// ClearFlag 清除标志位
func (m *Message) ClearFlag(flag uint8) {
	m.Flags &= ^flag
}

// HasFlag 检查是否设置了某标志位
func (m *Message) HasFlag(flag uint8) bool {
	return (m.Flags & flag) != 0
}

// Encode 将消息编码为字节数组
func (m *Message) Encode() ([]byte, error) {
	// 验证负载大小
	if len(m.Payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds maximum size %d", len(m.Payload), MaxPayloadSize)
	}

	// 验证服务名长度
	if len(m.SourceService) > MaxServiceNameSize {
		return nil, fmt.Errorf("source service name too long: %d > %d", len(m.SourceService), MaxServiceNameSize)
	}
	if len(m.DestinationService) > MaxServiceNameSize {
		return nil, fmt.Errorf("destination service name too long: %d > %d", len(m.DestinationService), MaxServiceNameSize)
	}

	// 计算额外字段长度
	sourceServiceLen := uint16(len(m.SourceService))
	destServiceLen := uint16(len(m.DestinationService))
	correlationIDLen := uint16(len(m.CorrelationID))
	sessionIDLen := uint16(len(m.SessionID))

	// 计算总长度
	totalLength := HeaderSize +
		2 + int(sourceServiceLen) + // 2字节长度 + 源服务名
		2 + int(destServiceLen) + // 2字节长度 + 目标服务名
		2 + int(correlationIDLen) + // 2字节长度 + 关联ID
		2 + int(sessionIDLen) + // 2字节长度 + 会话ID
		len(m.Payload) // 负载

	// 分配缓冲区
	buffer := make([]byte, totalLength)

	// 写入固定头部
	buffer[0] = m.Version
	buffer[1] = m.Flags
	binary.BigEndian.PutUint16(buffer[2:4], uint16(m.ServiceType))
	binary.BigEndian.PutUint16(buffer[4:6], uint16(m.MessageType))
	binary.BigEndian.PutUint32(buffer[6:10], m.SequenceID)
	binary.BigEndian.PutUint64(buffer[10:18], m.Timestamp)
	binary.BigEndian.PutUint32(buffer[18:22], m.PayloadLength)

	// 写入可变字段
	offset := HeaderSize

	// 源服务名
	binary.BigEndian.PutUint16(buffer[offset:offset+2], sourceServiceLen)
	offset += 2
	copy(buffer[offset:offset+int(sourceServiceLen)], m.SourceService)
	offset += int(sourceServiceLen)

	// 目标服务名
	binary.BigEndian.PutUint16(buffer[offset:offset+2], destServiceLen)
	offset += 2
	copy(buffer[offset:offset+int(destServiceLen)], m.DestinationService)
	offset += int(destServiceLen)

	// 关联ID
	binary.BigEndian.PutUint16(buffer[offset:offset+2], correlationIDLen)
	offset += 2
	copy(buffer[offset:offset+int(correlationIDLen)], m.CorrelationID)
	offset += int(correlationIDLen)

	// 会话ID
	binary.BigEndian.PutUint16(buffer[offset:offset+2], sessionIDLen)
	offset += 2
	copy(buffer[offset:offset+int(sessionIDLen)], m.SessionID)
	offset += int(sessionIDLen)

	// 写入负载
	copy(buffer[offset:], m.Payload)

	return buffer, nil
}

// Decode 从字节数组解码消息
func Decode(data []byte) (*Message, error) {
	// 验证数据长度
	if len(data) < HeaderSize {
		return nil, errors.New("data too short for message header")
	}

	m := &Message{}

	// 读取固定头部
	m.Version = data[0]
	m.Flags = data[1]
	m.ServiceType = ServiceType(binary.BigEndian.Uint16(data[2:4]))
	m.MessageType = MessageType(binary.BigEndian.Uint16(data[4:6]))
	m.SequenceID = binary.BigEndian.Uint32(data[6:10])
	m.Timestamp = binary.BigEndian.Uint64(data[10:18])
	m.PayloadLength = binary.BigEndian.Uint32(data[18:22])

	// 检查协议版本
	if m.Version != ProtocolVersion {
		return nil, fmt.Errorf("unsupported protocol version: %d", m.Version)
	}

	// 读取可变字段
	offset := HeaderSize

	// 源服务名
	if len(data) < offset+2 {
		return nil, errors.New("data too short for source service length")
	}
	sourceServiceLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	if len(data) < offset+int(sourceServiceLen) {
		return nil, errors.New("data too short for source service name")
	}
	m.SourceService = string(data[offset : offset+int(sourceServiceLen)])
	offset += int(sourceServiceLen)

	// 目标服务名
	if len(data) < offset+2 {
		return nil, errors.New("data too short for destination service length")
	}
	destServiceLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	if len(data) < offset+int(destServiceLen) {
		return nil, errors.New("data too short for destination service name")
	}
	m.DestinationService = string(data[offset : offset+int(destServiceLen)])
	offset += int(destServiceLen)

	// 关联ID
	if len(data) < offset+2 {
		return nil, errors.New("data too short for correlation ID length")
	}
	correlationIDLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	if len(data) < offset+int(correlationIDLen) {
		return nil, errors.New("data too short for correlation ID")
	}
	m.CorrelationID = string(data[offset : offset+int(correlationIDLen)])
	offset += int(correlationIDLen)

	// 会话ID
	if len(data) < offset+2 {
		return nil, errors.New("data too short for session ID length")
	}
	sessionIDLen := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	if len(data) < offset+int(sessionIDLen) {
		return nil, errors.New("data too short for session ID")
	}
	m.SessionID = string(data[offset : offset+int(sessionIDLen)])
	offset += int(sessionIDLen)

	// 读取负载
	if len(data) < offset+int(m.PayloadLength) {
		return nil, errors.New("data too short for payload")
	}
	m.Payload = data[offset : offset+int(m.PayloadLength)]

	return m, nil
}

// String 返回消息的字符串表示
func (m *Message) String() string {
	return fmt.Sprintf("Message{Version:%d, Flags:%#x, ServiceType:%d, MessageType:%d, SequenceID:%d, Src:%s, Dst:%s, SessionID:%s, PayloadLen:%d}",
		m.Version, m.Flags, m.ServiceType, m.MessageType, m.SequenceID, m.SourceService, m.DestinationService, m.SessionID, m.PayloadLength)
}

// ToJSON 将消息转换为JSON字符串（主要用于调试）
func (m *Message) ToJSON() (string, error) {
	type jsonMessage struct {
		Version            uint8       `json:"version"`
		Flags              uint8       `json:"flags"`
		ServiceType        ServiceType `json:"service_type"`
		MessageType        MessageType `json:"message_type"`
		SequenceID         uint32      `json:"sequence_id"`
		Timestamp          uint64      `json:"timestamp"`
		PayloadLength      uint32      `json:"payload_length"`
		SourceService      string      `json:"source_service,omitempty"`
		DestinationService string      `json:"destination_service,omitempty"`
		CorrelationID      string      `json:"correlation_id,omitempty"`
		SessionID          string      `json:"session_id,omitempty"`
		Payload            string      `json:"payload,omitempty"` // 转为Base64字符串
	}

	jm := jsonMessage{
		Version:            m.Version,
		Flags:              m.Flags,
		ServiceType:        m.ServiceType,
		MessageType:        m.MessageType,
		SequenceID:         m.SequenceID,
		Timestamp:          m.Timestamp,
		PayloadLength:      m.PayloadLength,
		SourceService:      m.SourceService,
		DestinationService: m.DestinationService,
		CorrelationID:      m.CorrelationID,
		SessionID:          m.SessionID,
		Payload:            fmt.Sprintf("<%d bytes>", len(m.Payload)),
	}

	jsonData, err := json.Marshal(jm)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// Clone 创建消息的深拷贝
func (m *Message) Clone() *Message {
	clone := &Message{
		Version:            m.Version,
		Flags:              m.Flags,
		ServiceType:        m.ServiceType,
		MessageType:        m.MessageType,
		SequenceID:         m.SequenceID,
		Timestamp:          m.Timestamp,
		PayloadLength:      m.PayloadLength,
		SourceService:      m.SourceService,
		DestinationService: m.DestinationService,
		CorrelationID:      m.CorrelationID,
		SessionID:          m.SessionID,
		Payload:            make([]byte, len(m.Payload)),
	}

	copy(clone.Payload, m.Payload)

	return clone
}
