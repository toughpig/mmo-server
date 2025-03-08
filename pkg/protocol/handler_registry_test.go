package protocol

import (
	"testing"
)

func TestNewHandlerRegistry(t *testing.T) {
	registry := NewHandlerRegistry()
	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}
	if registry.handlers == nil {
		t.Fatal("Expected non-nil handlers map")
	}
}

func TestRegisterHandler(t *testing.T) {
	registry := NewHandlerRegistry()

	// 定义测试处理器
	handler := func(msg *Message) (*Message, error) {
		return msg, nil
	}

	// 注册处理器
	err := registry.RegisterHandler(ServiceTypeAuth, 123, handler)
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// 尝试重复注册
	err = registry.RegisterHandler(ServiceTypeAuth, 123, handler)
	if err == nil {
		t.Fatal("Expected error when registering duplicate handler, got nil")
	}

	// 注册不同消息类型
	err = registry.RegisterHandler(ServiceTypeAuth, 456, handler)
	if err != nil {
		t.Fatalf("Failed to register handler for different message type: %v", err)
	}

	// 注册不同服务类型
	err = registry.RegisterHandler(ServiceTypeChat, 123, handler)
	if err != nil {
		t.Fatalf("Failed to register handler for different service type: %v", err)
	}
}

func TestGetHandler(t *testing.T) {
	registry := NewHandlerRegistry()

	// 定义测试处理器
	testHandler := func(msg *Message) (*Message, error) {
		// 修改消息以便测试时确认是否调用了正确的处理器
		msg.SourceService = "test-handler"
		return msg, nil
	}

	// 注册处理器
	err := registry.RegisterHandler(ServiceTypeAuth, 123, testHandler)
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// 获取处理器
	handler, err := registry.GetHandler(ServiceTypeAuth, 123)
	if err != nil {
		t.Fatalf("Failed to get handler: %v", err)
	}

	if handler == nil {
		t.Fatal("Retrieved handler is nil")
	}

	// 测试处理器功能
	msg := NewMessage(ServiceTypeAuth, 123, []byte("test"))
	response, err := handler(msg)
	if err != nil {
		t.Fatalf("Handler execution failed: %v", err)
	}

	if response.SourceService != "test-handler" {
		t.Errorf("Expected source service 'test-handler', got '%s'", response.SourceService)
	}

	// 测试获取未注册的处理器
	_, err = registry.GetHandler(ServiceTypeGame, 999)
	if err == nil {
		t.Fatal("Expected error when getting unregistered handler, got nil")
	}
}

func TestUnregisterHandler(t *testing.T) {
	registry := NewHandlerRegistry()

	// 定义测试处理器
	handler := func(msg *Message) (*Message, error) {
		return msg, nil
	}

	// 注册处理器
	serviceType := ServiceTypeAuth
	messageType := MessageType(123)

	err := registry.RegisterHandler(serviceType, messageType, handler)
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// 测试注销处理器
	err = registry.UnregisterHandler(serviceType, messageType)
	if err != nil {
		t.Fatalf("Failed to unregister handler: %v", err)
	}

	// 验证处理器已被注销
	_, err = registry.GetHandler(serviceType, messageType)
	if err == nil {
		t.Fatal("Expected error after unregistering handler, got nil")
	}

	// 测试注销未注册的处理器
	err = registry.UnregisterHandler(serviceType, messageType)
	if err == nil {
		t.Fatal("Expected error when unregistering non-existent handler, got nil")
	}
}

func TestListHandlers(t *testing.T) {
	registry := NewHandlerRegistry()
	handler := func(msg *Message) (*Message, error) {
		return msg, nil
	}

	// 注册多个处理器
	err := registry.RegisterHandler(ServiceTypeAuth, 123, handler)
	if err != nil {
		t.Fatalf("Failed to register handler 1: %v", err)
	}

	err = registry.RegisterHandler(ServiceTypeAuth, 456, handler)
	if err != nil {
		t.Fatalf("Failed to register handler 2: %v", err)
	}

	err = registry.RegisterHandler(ServiceTypeChat, 789, handler)
	if err != nil {
		t.Fatalf("Failed to register handler 3: %v", err)
	}

	// 获取处理器列表
	handlerList := registry.ListHandlers()

	// 验证结果
	if len(handlerList) != 2 {
		t.Fatalf("Expected 2 service types, got %d", len(handlerList))
	}

	authHandlers, ok := handlerList[ServiceTypeAuth]
	if !ok {
		t.Fatal("Auth service type not found in handler list")
	}

	if len(authHandlers) != 2 {
		t.Fatalf("Expected 2 auth handlers, got %d", len(authHandlers))
	}

	chatHandlers, ok := handlerList[ServiceTypeChat]
	if !ok {
		t.Fatal("Chat service type not found in handler list")
	}

	if len(chatHandlers) != 1 {
		t.Fatalf("Expected 1 chat handler, got %d", len(chatHandlers))
	}
}

func TestHasHandler(t *testing.T) {
	registry := NewHandlerRegistry()
	handler := func(msg *Message) (*Message, error) {
		return msg, nil
	}

	// 注册处理器
	serviceType := ServiceTypeAuth
	messageType := MessageType(123)

	err := registry.RegisterHandler(serviceType, messageType, handler)
	if err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// 测试HasHandler
	if !registry.HasHandler(serviceType, messageType) {
		t.Error("HasHandler returned false for registered handler")
	}

	if registry.HasHandler(ServiceTypeGame, 999) {
		t.Error("HasHandler returned true for unregistered handler")
	}

	// 注销处理器后再次测试
	err = registry.UnregisterHandler(serviceType, messageType)
	if err != nil {
		t.Fatalf("Failed to unregister handler: %v", err)
	}

	if registry.HasHandler(serviceType, messageType) {
		t.Error("HasHandler returned true after handler was unregistered")
	}
}
