package gateway

import (
	"context"
	"mmo-server/pkg/protocol"
	"time"
)

// MessageHandler 定义消息处理器接口
type MessageHandler interface {
	// HandleMessage 处理消息并返回响应
	HandleMessage(msg *protocol.Message) (*protocol.Message, error)
}

// 实现GatewayHandler的MessageHandler接口
// 这个文件将被gateway_handler.go中的实现替代，保留它只是为了兼容性
func (h *GatewayHandler) HandleMessage(msg *protocol.Message) (*protocol.Message, error) {
	// 使用路由器处理消息
	if h.router == nil {
		return nil, nil
	}

	// 使用带超时的上下文（默认5秒超时）
	timeout := 5 * time.Second // 使用固定超时，因为h.routeTimeout不可用
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 通过路由器路由消息
	return h.router.Route(ctx, msg)
}
