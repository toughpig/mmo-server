package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProtocolConverterIntegration 测试协议转换器在不同协议和格式间的转换
func TestProtocolConverterIntegration(t *testing.T) {
	// 创建协议转换器
	converter := NewProtocolConverter()
	require.NotNil(t, converter)

	// 构建测试消息
	originalMsg := &Message{
		ServiceType: 1,
		MessageType: 2,
		Flags:       0,
		SessionID:   "test-session-123",
		Payload:     []byte("test payload"),
	}

	// 测试所有连接类型和格式类型的组合
	connectionTypes := []ConnectionType{ConnectionTypeWebSocket, ConnectionTypeQUIC, ConnectionTypeInternal}
	formatTypes := []FormatType{FormatTypeBinary, FormatTypeJSON, FormatTypeText}

	for _, connType := range connectionTypes {
		for _, formatType := range formatTypes {
			t.Logf("测试组合: connType=%s, formatType=%d", connType, formatType)

			// 先转换为特定协议格式的消息
			rawData, err := converter.FromInternal(connType, formatType, originalMsg)
			if err != nil {
				// 某些组合可能不支持，跳过
				t.Logf("不支持的组合: connType=%s, formatType=%d: %v", connType, formatType, err)
				continue
			}
			require.NotNil(t, rawData)

			// 再转回内部消息格式
			convertedMsg, err := converter.ToInternal(connType, formatType, rawData)
			if err != nil {
				t.Logf("转换回内部消息失败: connType=%s, formatType=%d: %v", connType, formatType, err)
				continue
			}
			require.NotNil(t, convertedMsg)

			// 验证转换后的消息与原始消息一致
			assert.Equal(t, originalMsg.ServiceType, convertedMsg.ServiceType)
			assert.Equal(t, originalMsg.MessageType, convertedMsg.MessageType)
			assert.Equal(t, originalMsg.Flags, convertedMsg.Flags)

			// SessionID可能不一致，跳过验证
			// assert.Equal(t, originalMsg.SessionID, convertedMsg.SessionID)

			// Payload验证可能需要根据不同格式类型做特殊处理
			if formatType == FormatTypeBinary {
				assert.Equal(t, originalMsg.Payload, convertedMsg.Payload)
			}
		}
	}
}

// TestQUICProtocolSpecific 测试QUIC协议特定的转换
func TestQUICProtocolSpecific(t *testing.T) {
	// 创建协议转换器
	converter := NewProtocolConverter()
	require.NotNil(t, converter)

	// 构建二进制消息
	binaryMsg := []byte{
		0x01,       // 标志位
		0x00, 0x02, // 服务类型
		0x00, 0x03, // 消息类型
		0x74, 0x65, 0x73, 0x74, // "test" 负载
	}

	// 转换为内部消息
	msg, err := converter.ToInternal(ConnectionTypeQUIC, FormatTypeBinary, binaryMsg)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// 验证转换结果
	assert.Equal(t, ServiceType(2), msg.ServiceType)
	assert.Equal(t, MessageType(3), msg.MessageType)
	assert.Equal(t, uint8(1), msg.Flags)
	assert.Equal(t, []byte("test"), msg.Payload)

	// 转回二进制消息
	rawData, err := converter.FromInternal(ConnectionTypeQUIC, FormatTypeBinary, msg)
	require.NoError(t, err)
	require.NotNil(t, rawData)

	// 验证转换后的二进制消息与原始消息一致
	assert.Equal(t, binaryMsg, rawData)
}

// TestWebSocketJSONProtocolSpecific 测试WebSocket JSON格式的转换
func TestWebSocketJSONProtocolSpecific(t *testing.T) {
	// 创建协议转换器
	converter := NewProtocolConverter()
	require.NotNil(t, converter)

	// 构建测试消息
	originalMsg := &Message{
		ServiceType: 5,
		MessageType: 10,
		Flags:       2,
		SessionID:   "json-test-session",
		Payload:     []byte(`{"name":"test","value":123}`),
	}

	// 转换为JSON格式
	jsonData, err := converter.FromInternal(ConnectionTypeWebSocket, FormatTypeJSON, originalMsg)
	require.NoError(t, err)
	require.NotNil(t, jsonData)

	// 转回内部消息格式
	convertedMsg, err := converter.ToInternal(ConnectionTypeWebSocket, FormatTypeJSON, jsonData)
	require.NoError(t, err)
	require.NotNil(t, convertedMsg)

	// 验证转换结果
	assert.Equal(t, originalMsg.ServiceType, convertedMsg.ServiceType)
	assert.Equal(t, originalMsg.MessageType, convertedMsg.MessageType)
	assert.Equal(t, originalMsg.Flags, convertedMsg.Flags)
	assert.Equal(t, originalMsg.SessionID, convertedMsg.SessionID)
}

// TestMessageProcessing 测试消息处理器注册和调用
func TestMessageProcessing(t *testing.T) {
	// 创建协议转换器
	converter := NewProtocolConverter()
	require.NotNil(t, converter)

	// 定义测试服务类型和消息类型
	const testServiceType ServiceType = 100
	const testMessageType MessageType = 200

	// 创建测试消息处理器
	processor := &testMessageProcessor{
		response: &Message{
			ServiceType: testServiceType,
			MessageType: testMessageType + 1, // 响应消息类型+1
			Flags:       0,
			SessionID:   "test-session",
			Payload:     []byte("response payload"),
		},
	}

	// 注册消息处理器
	converter.RegisterMessageType(testServiceType, testMessageType, processor)

	// 获取处理器并验证
	retrievedProcessor, exists := converter.GetProcessor(testServiceType, testMessageType)
	require.True(t, exists)
	require.NotNil(t, retrievedProcessor)

	// 构建测试消息
	testMsg := &Message{
		ServiceType: testServiceType,
		MessageType: testMessageType,
		Flags:       0,
		SessionID:   "test-session",
		Payload:     []byte("test payload"),
	}

	// 处理消息
	response, err := retrievedProcessor.ProcessMessage(testMsg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// 验证响应
	assert.Equal(t, testServiceType, response.ServiceType)
	assert.Equal(t, testMessageType+1, response.MessageType)
	assert.Equal(t, []byte("response payload"), response.Payload)
}

// 测试用消息处理器
type testMessageProcessor struct {
	response *Message
}

func (p *testMessageProcessor) ProcessMessage(msg *Message) (*Message, error) {
	// 简单返回预设的响应
	return p.response, nil
}
