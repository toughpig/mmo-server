package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/yourusername/mmo-server/pkg/protocol"
)

// 测试处理器实现
type EchoProcessor struct{}

func (p *EchoProcessor) ProcessMessage(msg *protocol.Message) (*protocol.Message, error) {
	// 简单回显处理器：返回相同的消息，但添加时间戳
	response := msg.Clone()
	response.Timestamp = uint64(time.Now().UnixMicro())

	// 如果有源服务和目标服务，交换它们
	response.SourceService, response.DestinationService = msg.DestinationService, msg.SourceService

	log.Printf("Processed message: %s, payload: %s", msg.String(), string(msg.Payload))
	return response, nil
}

func main() {
	log.Println("协议转换测试")

	// 创建协议转换器
	converter := protocol.NewProtocolConverter()

	// 注册测试消息处理器
	converter.RegisterMessageType(
		protocol.ServiceTypeChat,
		1, // 聊天消息类型
		&EchoProcessor{},
	)

	// 创建路由器
	router := protocol.NewMessageRouter(
		protocol.NewInMemoryServiceDiscovery(),
		protocol.NewRoundRobinLoadBalancer(),
		converter,
		protocol.NewHandlerRegistry(),
	)

	// 注册服务类型映射
	router.RegisterServiceType(protocol.ServiceTypeChat, "chat-service")

	// 创建测试消息
	testMessage := protocol.NewMessage(
		protocol.ServiceTypeChat,
		1, // 聊天消息类型
		[]byte(`{"content":"Hello, World!", "from":"user1", "to":"all"}`),
	)
	testMessage.SourceService = "client"
	testMessage.DestinationService = "chat-service"
	testMessage.SessionID = "test-session-123"

	// 测试不同格式的转换
	testFormatConversion(converter, testMessage)

	// 测试消息路由
	testMessageRouting(router, testMessage)
}

// 测试不同格式的转换
func testFormatConversion(converter protocol.ProtocolConverter, msg *protocol.Message) {
	log.Println("\n=== 测试格式转换 ===")

	// 将消息编码为二进制格式
	binaryData, err := msg.Encode()
	if err != nil {
		log.Fatalf("Failed to encode message: %v", err)
	}
	log.Printf("Binary message size: %d bytes", len(binaryData))

	// 转换回内部格式
	decodedMsg, err := protocol.Decode(binaryData)
	if err != nil {
		log.Fatalf("Failed to decode message: %v", err)
	}
	log.Printf("Decoded message: %s", decodedMsg.String())

	// WebSocket格式转换：JSON
	jsonData, err := converter.FromInternal(protocol.ConnectionTypeWebSocket, protocol.FormatTypeJSON, msg)
	if err != nil {
		log.Fatalf("Failed to convert to JSON: %v", err)
	}
	log.Printf("JSON message: %s", string(jsonData))

	// JSON格式转回内部消息
	internalMsgFromJSON, err := converter.ToInternal(protocol.ConnectionTypeWebSocket, protocol.FormatTypeJSON, jsonData)
	if err != nil {
		log.Fatalf("Failed to convert from JSON: %v", err)
	}
	log.Printf("Internal message from JSON: %s", internalMsgFromJSON.String())
}

// 测试消息路由
func testMessageRouting(router *protocol.MessageRouter, msg *protocol.Message) {
	log.Println("\n=== 测试消息路由 ===")

	// 注册测试服务实例
	serviceDiscovery := protocol.NewInMemoryServiceDiscovery()
	instance := protocol.ServiceInstance{
		ID:          "chat-service-1",
		ServiceName: "chat-service",
		Address:     "localhost:8080",
		Weight:      100,
		Status:      "up",
		LastSeen:    time.Now(),
	}
	err := serviceDiscovery.RegisterServiceInstance(instance)
	if err != nil {
		log.Fatalf("Failed to register service instance: %v", err)
	}

	// 创建新的路由器，使用我们的服务发现实例
	registry := protocol.NewHandlerRegistry()
	registry.RegisterHandler(protocol.ServiceTypeChat, 1, func(m *protocol.Message) (*protocol.Message, error) {
		response := m.Clone()
		response.SourceService, response.DestinationService = m.DestinationService, m.SourceService
		response.Payload = []byte(`{"status":"ok", "message":"Message received"}`)
		log.Printf("Handler processed message: %s", m.String())
		return response, nil
	})

	// 创建新的协议转换器
	converter := protocol.NewProtocolConverter()

	newRouter := protocol.NewMessageRouter(
		serviceDiscovery,
		protocol.NewRoundRobinLoadBalancer(),
		converter,
		registry,
	)

	// 注册服务类型映射
	newRouter.RegisterServiceType(protocol.ServiceTypeChat, "chat-service")

	// 创建路由上下文
	ctx := context.Background()

	// 路由消息
	response, err := newRouter.Route(ctx, msg)
	if err != nil {
		log.Fatalf("Failed to route message: %v", err)
	}

	// 输出响应
	if response != nil {
		log.Printf("Response received: %s", response.String())
		payloadStr, _ := response.ToJSON()
		log.Printf("Response payload: %s", payloadStr)
	} else {
		log.Println("No response received")
	}
}

// 打印消息内容
func printMessage(msg *protocol.Message) {
	jsonMsg, _ := json.MarshalIndent(msg, "", "  ")
	fmt.Println(string(jsonMsg))
}
