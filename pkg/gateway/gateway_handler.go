package gateway

import (
	"context"
	"time"

	"mmo-server/pkg/protocol"
)

// SimpleMessageHandler 实现简单的MessageHandler接口，负责消息路由
type SimpleMessageHandler struct {
	router       *protocol.MessageRouter
	routeTimeout time.Duration
}

// NewSimpleMessageHandler 创建新的简单消息处理器
func NewSimpleMessageHandler(router *protocol.MessageRouter) *SimpleMessageHandler {
	return &SimpleMessageHandler{
		router:       router,
		routeTimeout: 5 * time.Second, // 默认5秒超时
	}
}

// HandleMessage 处理消息并返回响应
func (h *SimpleMessageHandler) HandleMessage(msg *protocol.Message) (*protocol.Message, error) {
	// 使用路由器处理消息
	if h.router == nil {
		return nil, nil
	}

	// 使用带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), h.routeTimeout)
	defer cancel()

	// 通过路由器路由消息
	return h.router.Route(ctx, msg)
}

// SetRouteTimeout 设置路由超时时间
func (h *SimpleMessageHandler) SetRouteTimeout(timeout time.Duration) {
	h.routeTimeout = timeout
}
