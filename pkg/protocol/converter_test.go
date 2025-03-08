package protocol

import (
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"
)

// MockMessageProcessor 用于测试的消息处理器
type MockMessageProcessor struct {
	ProcessFunc func(msg *Message) (*Message, error)
}

func (p *MockMessageProcessor) ProcessMessage(msg *Message) (*Message, error) {
	if p.ProcessFunc != nil {
		return p.ProcessFunc(msg)
	}
	return msg, nil
}

func TestNewProtocolConverter(t *testing.T) {
	converter := NewProtocolConverter()
	if converter == nil {
		t.Fatal("Expected non-nil converter")
	}
	if converter.processors == nil {
		t.Fatal("Expected non-nil processors map")
	}
}

func TestRegisterMessageType(t *testing.T) {
	converter := NewProtocolConverter()
	processor := &MockMessageProcessor{}

	// 注册消息类型
	serviceType := ServiceTypeAuth
	messageType := MessageType(123)
	converter.RegisterMessageType(serviceType, messageType, processor)

	// 验证注册是否成功
	registeredProcessor, exists := converter.GetProcessor(serviceType, messageType)
	if !exists {
		t.Fatal("Expected processor to be registered")
	}

	if registeredProcessor != processor {
		t.Fatal("Registered processor does not match the original")
	}
}

func TestWebSocketBinaryToInternal(t *testing.T) {
	converter := NewProtocolConverter()

	// 创建WebSocket二进制消息
	// 格式：1字节消息类型 + 2字节服务类型 + 2字节消息类型 + 负载
	msgFlag := uint8(FlagReliable)
	serviceType := ServiceTypeAuth
	messageType := MessageType(123)
	payload := []byte("test payload")

	rawMsg := make([]byte, 5+len(payload))
	rawMsg[0] = msgFlag
	binary.BigEndian.PutUint16(rawMsg[1:3], uint16(serviceType))
	binary.BigEndian.PutUint16(rawMsg[3:5], uint16(messageType))
	copy(rawMsg[5:], payload)

	// 转换为内部格式
	internalMsg, err := converter.ToInternal(ConnectionTypeWebSocket, FormatTypeBinary, rawMsg)
	if err != nil {
		t.Fatalf("Failed to convert to internal format: %v", err)
	}

	// 验证转换结果
	if internalMsg.Flags != msgFlag {
		t.Errorf("Expected flags %v, got %v", msgFlag, internalMsg.Flags)
	}

	if internalMsg.ServiceType != serviceType {
		t.Errorf("Expected service type %v, got %v", serviceType, internalMsg.ServiceType)
	}

	if internalMsg.MessageType != messageType {
		t.Errorf("Expected message type %v, got %v", messageType, internalMsg.MessageType)
	}

	if !reflect.DeepEqual(internalMsg.Payload, payload) {
		t.Errorf("Expected payload %v, got %v", payload, internalMsg.Payload)
	}
}

func TestWebSocketJSONToInternal(t *testing.T) {
	converter := NewProtocolConverter()

	// 创建WebSocket JSON消息
	jsonMsg := struct {
		ServiceType uint16 `json:"service_type"`
		MessageType uint16 `json:"message_type"`
		Flags       uint8  `json:"flags"`
		Payload     []byte `json:"payload"`
		SessionID   string `json:"session_id,omitempty"`
	}{
		ServiceType: uint16(ServiceTypeChat),
		MessageType: 456,
		Flags:       FlagEncrypted,
		Payload:     []byte("json payload"),
		SessionID:   "test-session",
	}

	jsonData, err := json.Marshal(jsonMsg)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	// 转换为内部格式
	internalMsg, err := converter.ToInternal(ConnectionTypeWebSocket, FormatTypeJSON, jsonData)
	if err != nil {
		t.Fatalf("Failed to convert to internal format: %v", err)
	}

	// 验证转换结果
	if internalMsg.Flags != jsonMsg.Flags {
		t.Errorf("Expected flags %v, got %v", jsonMsg.Flags, internalMsg.Flags)
	}

	if internalMsg.ServiceType != ServiceType(jsonMsg.ServiceType) {
		t.Errorf("Expected service type %v, got %v", jsonMsg.ServiceType, internalMsg.ServiceType)
	}

	if internalMsg.MessageType != MessageType(jsonMsg.MessageType) {
		t.Errorf("Expected message type %v, got %v", jsonMsg.MessageType, internalMsg.MessageType)
	}

	if internalMsg.SessionID != jsonMsg.SessionID {
		t.Errorf("Expected session ID %v, got %v", jsonMsg.SessionID, internalMsg.SessionID)
	}
}

func TestInternalToWebSocketBinary(t *testing.T) {
	converter := NewProtocolConverter()

	// 创建内部消息
	msg := NewMessage(ServiceTypeGame, 789, []byte("internal payload"))
	msg.SetFlag(FlagReliable | FlagOrdered)

	// 转换为WebSocket二进制格式
	rawData, err := converter.FromInternal(ConnectionTypeWebSocket, FormatTypeBinary, msg)
	if err != nil {
		t.Fatalf("Failed to convert from internal format: %v", err)
	}

	// 验证转换结果
	if len(rawData) < 5 {
		t.Fatalf("Raw data too short: %d bytes", len(rawData))
	}

	if rawData[0] != msg.Flags {
		t.Errorf("Expected flags %v, got %v", msg.Flags, rawData[0])
	}

	serviceType := ServiceType(binary.BigEndian.Uint16(rawData[1:3]))
	if serviceType != msg.ServiceType {
		t.Errorf("Expected service type %v, got %v", msg.ServiceType, serviceType)
	}

	messageType := MessageType(binary.BigEndian.Uint16(rawData[3:5]))
	if messageType != msg.MessageType {
		t.Errorf("Expected message type %v, got %v", msg.MessageType, messageType)
	}

	payload := rawData[5:]
	if !reflect.DeepEqual(payload, msg.Payload) {
		t.Errorf("Expected payload %v, got %v", msg.Payload, payload)
	}
}

func TestRoundTrip(t *testing.T) {
	converter := NewProtocolConverter()

	// 创建内部消息
	originalMsg := NewMessage(ServiceTypeInventory, 321, []byte("round trip test"))
	originalMsg.SetFlag(FlagEncrypted)
	originalMsg.SessionID = "session-123"
	originalMsg.SourceService = "client"
	originalMsg.DestinationService = "inventory-service"

	// 内部 -> WebSocket
	wsData, err := converter.FromInternal(ConnectionTypeWebSocket, FormatTypeJSON, originalMsg)
	if err != nil {
		t.Fatalf("Failed to convert to WebSocket: %v", err)
	}

	// WebSocket -> 内部
	resultMsg, err := converter.ToInternal(ConnectionTypeWebSocket, FormatTypeJSON, wsData)
	if err != nil {
		t.Fatalf("Failed to convert back to internal: %v", err)
	}

	// 验证转换后的消息与原始消息一致
	if resultMsg.Flags != originalMsg.Flags {
		t.Errorf("Flags don't match: %v vs %v", resultMsg.Flags, originalMsg.Flags)
	}

	if resultMsg.ServiceType != originalMsg.ServiceType {
		t.Errorf("Service type doesn't match: %v vs %v", resultMsg.ServiceType, originalMsg.ServiceType)
	}

	if resultMsg.MessageType != originalMsg.MessageType {
		t.Errorf("Message type doesn't match: %v vs %v", resultMsg.MessageType, originalMsg.MessageType)
	}

	if resultMsg.SessionID != originalMsg.SessionID {
		t.Errorf("Session ID doesn't match: %v vs %v", resultMsg.SessionID, originalMsg.SessionID)
	}

	if !reflect.DeepEqual(resultMsg.Payload, originalMsg.Payload) {
		t.Errorf("Payload doesn't match: %v vs %v", resultMsg.Payload, originalMsg.Payload)
	}
}

func TestInvalidFormat(t *testing.T) {
	converter := NewProtocolConverter()

	// 测试数据过短
	shortData := []byte{0x01, 0x02}
	_, err := converter.ToInternal(ConnectionTypeWebSocket, FormatTypeBinary, shortData)
	if err == nil {
		t.Errorf("Expected error for short data, got nil")
	}

	// 测试不支持的连接类型
	_, err = converter.ToInternal("UnsupportedConnection", FormatTypeBinary, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06})
	if err == nil {
		t.Errorf("Expected error for unsupported connection type, got nil")
	}

	// 测试不支持的格式类型
	_, err = converter.ToInternal(ConnectionTypeWebSocket, FormatType(99), []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06})
	if err == nil {
		t.Errorf("Expected error for unsupported format type, got nil")
	}
}
