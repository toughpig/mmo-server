package protocol

import (
	"fmt"
	"sync"
)

// MessageHandlerFunc 消息处理器函数类型
type MessageHandlerFunc func(msg *Message) (*Message, error)

// HandlerRegistry 消息处理器注册表，管理消息类型和处理器的映射
type HandlerRegistry struct {
	handlers map[ServiceType]map[MessageType]MessageHandlerFunc
	mu       sync.RWMutex
}

// NewHandlerRegistry 创建消息处理器注册表
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[ServiceType]map[MessageType]MessageHandlerFunc),
	}
}

// RegisterHandler 注册消息处理器
func (r *HandlerRegistry) RegisterHandler(serviceType ServiceType, messageType MessageType, handler MessageHandlerFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查服务类型是否已注册
	if _, ok := r.handlers[serviceType]; !ok {
		r.handlers[serviceType] = make(map[MessageType]MessageHandlerFunc)
	}

	// 检查消息类型是否已注册
	if _, ok := r.handlers[serviceType][messageType]; ok {
		return fmt.Errorf("handler already registered for service type %d, message type %d", serviceType, messageType)
	}

	// 注册处理器
	r.handlers[serviceType][messageType] = handler
	return nil
}

// GetHandler 获取消息处理器
func (r *HandlerRegistry) GetHandler(serviceType ServiceType, messageType MessageType) (MessageHandlerFunc, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 检查服务类型是否已注册
	serviceHandlers, ok := r.handlers[serviceType]
	if !ok {
		return nil, fmt.Errorf("no handlers registered for service type %d", serviceType)
	}

	// 检查消息类型是否已注册
	handler, ok := serviceHandlers[messageType]
	if !ok {
		return nil, fmt.Errorf("no handler registered for service type %d, message type %d", serviceType, messageType)
	}

	return handler, nil
}

// UnregisterHandler 注销消息处理器
func (r *HandlerRegistry) UnregisterHandler(serviceType ServiceType, messageType MessageType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查服务类型是否已注册
	serviceHandlers, ok := r.handlers[serviceType]
	if !ok {
		return fmt.Errorf("no handlers registered for service type %d", serviceType)
	}

	// 检查消息类型是否已注册
	if _, ok := serviceHandlers[messageType]; !ok {
		return fmt.Errorf("no handler registered for service type %d, message type %d", serviceType, messageType)
	}

	// 注销处理器
	delete(serviceHandlers, messageType)

	// 如果服务类型下没有处理器了，删除服务类型
	if len(serviceHandlers) == 0 {
		delete(r.handlers, serviceType)
	}

	return nil
}

// ListHandlers 列出所有已注册的处理器
func (r *HandlerRegistry) ListHandlers() map[ServiceType][]MessageType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[ServiceType][]MessageType)

	for serviceType, serviceHandlers := range r.handlers {
		messageTypes := make([]MessageType, 0, len(serviceHandlers))
		for messageType := range serviceHandlers {
			messageTypes = append(messageTypes, messageType)
		}
		result[serviceType] = messageTypes
	}

	return result
}

// HasHandler 检查是否有处理器注册
func (r *HandlerRegistry) HasHandler(serviceType ServiceType, messageType MessageType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	serviceHandlers, ok := r.handlers[serviceType]
	if !ok {
		return false
	}

	_, ok = serviceHandlers[messageType]
	return ok
}
