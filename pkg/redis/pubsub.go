package redis

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// MessageHandler 是消息处理函数的类型
type MessageHandler func(channel string, payload []byte)

// PubSub 是Redis发布订阅服务的接口
type PubSub interface {
	// 发布消息到指定频道
	Publish(ctx context.Context, channel string, message interface{}) error
	// 订阅一个或多个频道
	Subscribe(ctx context.Context, handler MessageHandler, channels ...string) error
	// 取消订阅一个或多个频道
	Unsubscribe(ctx context.Context, channels ...string) error
	// 订阅匹配模式的频道
	PSubscribe(ctx context.Context, handler MessageHandler, patterns ...string) error
	// 取消订阅匹配模式的频道
	PUnsubscribe(ctx context.Context, patterns ...string) error
	// 获取活跃的频道数量
	NumActiveChannels() int
	// 获取订阅客户端数量
	NumSubscribers(ctx context.Context, channel string) (int64, error)
	// 获取所有活跃频道
	ActiveChannels() []string
	// 关闭发布订阅服务
	Close() error
}

// RedisPubSub 实现Redis发布订阅服务
type RedisPubSub struct {
	pool            Pool
	pubsub          *redis.PubSub
	handlers        map[string][]MessageHandler
	patternHandlers map[string][]MessageHandler
	activeChannels  map[string]struct{}
	mu              sync.RWMutex
	isRunning       bool
	stopChan        chan struct{}
}

// NewPubSub 创建新的Redis发布订阅服务
func NewPubSub(pool Pool) (PubSub, error) {
	if pool == nil {
		return nil, errors.New("Redis连接池未初始化")
	}

	ps := &RedisPubSub{
		pool:            pool,
		handlers:        make(map[string][]MessageHandler),
		patternHandlers: make(map[string][]MessageHandler),
		activeChannels:  make(map[string]struct{}),
		stopChan:        make(chan struct{}),
	}

	// 验证Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := pool.GetClient()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis连接验证失败: %w", err)
	}

	log.Printf("Redis发布订阅服务已初始化")
	return ps, nil
}

// Publish 发布消息到指定频道
func (ps *RedisPubSub) Publish(ctx context.Context, channel string, message interface{}) error {
	client := ps.pool.GetClient()

	var msgStr string
	switch msg := message.(type) {
	case string:
		msgStr = msg
	case []byte:
		msgStr = string(msg)
	default:
		msgStr = fmt.Sprintf("%v", msg)
	}

	result, err := client.Publish(ctx, channel, msgStr).Result()
	if err != nil {
		return fmt.Errorf("发布消息失败: %w", err)
	}

	log.Printf("消息已发布到频道 %s，接收者数量: %d", channel, result)
	return nil
}

// Subscribe 订阅一个或多个频道
func (ps *RedisPubSub) Subscribe(ctx context.Context, handler MessageHandler, channels ...string) error {
	if len(channels) == 0 {
		return errors.New("未指定订阅频道")
	}

	if handler == nil {
		return errors.New("消息处理函数不能为nil")
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 如果尚未创建pubsub对象，则创建一个
	if ps.pubsub == nil {
		client := ps.pool.GetClient()
		if redisClient, ok := client.(*redis.Client); ok {
			ps.pubsub = redisClient.Subscribe(ctx)
		} else if clusterClient, ok := client.(*redis.ClusterClient); ok {
			ps.pubsub = clusterClient.Subscribe(ctx)
		} else {
			return errors.New("不支持的Redis客户端类型")
		}

		// 启动消息接收处理循环
		ps.startMessageLoop(ctx)
	}

	// 注册处理函数
	for _, channel := range channels {
		ps.handlers[channel] = append(ps.handlers[channel], handler)
		ps.activeChannels[channel] = struct{}{}
	}

	// 订阅频道
	err := ps.pubsub.Subscribe(ctx, channels...)
	if err != nil {
		return fmt.Errorf("订阅频道失败: %w", err)
	}

	log.Printf("已订阅频道: %v", channels)
	return nil
}

// Unsubscribe 取消订阅一个或多个频道
func (ps *RedisPubSub) Unsubscribe(ctx context.Context, channels ...string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.pubsub == nil {
		return errors.New("未初始化发布订阅客户端")
	}

	// 取消订阅频道
	err := ps.pubsub.Unsubscribe(ctx, channels...)
	if err != nil {
		return fmt.Errorf("取消订阅频道失败: %w", err)
	}

	// 移除处理函数
	for _, channel := range channels {
		delete(ps.handlers, channel)
		delete(ps.activeChannels, channel)
	}

	log.Printf("已取消订阅频道: %v", channels)
	return nil
}

// PSubscribe 订阅匹配模式的频道
func (ps *RedisPubSub) PSubscribe(ctx context.Context, handler MessageHandler, patterns ...string) error {
	if len(patterns) == 0 {
		return errors.New("未指定订阅模式")
	}

	if handler == nil {
		return errors.New("消息处理函数不能为nil")
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 如果尚未创建pubsub对象，则创建一个
	if ps.pubsub == nil {
		client := ps.pool.GetClient()
		if redisClient, ok := client.(*redis.Client); ok {
			ps.pubsub = redisClient.Subscribe(ctx)
		} else if clusterClient, ok := client.(*redis.ClusterClient); ok {
			ps.pubsub = clusterClient.Subscribe(ctx)
		} else {
			return errors.New("不支持的Redis客户端类型")
		}

		// 启动消息接收处理循环
		ps.startMessageLoop(ctx)
	}

	// 注册处理函数
	for _, pattern := range patterns {
		ps.patternHandlers[pattern] = append(ps.patternHandlers[pattern], handler)
	}

	// 订阅模式
	err := ps.pubsub.PSubscribe(ctx, patterns...)
	if err != nil {
		return fmt.Errorf("订阅模式失败: %w", err)
	}

	log.Printf("已订阅模式: %v", patterns)
	return nil
}

// PUnsubscribe 取消订阅匹配模式的频道
func (ps *RedisPubSub) PUnsubscribe(ctx context.Context, patterns ...string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.pubsub == nil {
		return errors.New("未初始化发布订阅客户端")
	}

	// 取消订阅模式
	err := ps.pubsub.PUnsubscribe(ctx, patterns...)
	if err != nil {
		return fmt.Errorf("取消订阅模式失败: %w", err)
	}

	// 移除处理函数
	for _, pattern := range patterns {
		delete(ps.patternHandlers, pattern)
	}

	log.Printf("已取消订阅模式: %v", patterns)
	return nil
}

// NumActiveChannels 获取活跃的频道数量
func (ps *RedisPubSub) NumActiveChannels() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.activeChannels)
}

// NumSubscribers 获取订阅客户端数量
func (ps *RedisPubSub) NumSubscribers(ctx context.Context, channel string) (int64, error) {
	client := ps.pool.GetClient()

	result, err := client.PubSubNumSub(ctx, channel).Result()
	if err != nil {
		return 0, fmt.Errorf("获取订阅者数量失败: %w", err)
	}

	return result[channel], nil
}

// ActiveChannels 获取所有活跃频道
func (ps *RedisPubSub) ActiveChannels() []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	channels := make([]string, 0, len(ps.activeChannels))
	for channel := range ps.activeChannels {
		channels = append(channels, channel)
	}

	return channels
}

// Close 关闭发布订阅服务
func (ps *RedisPubSub) Close() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if !ps.isRunning {
		return nil
	}

	// 停止消息循环
	close(ps.stopChan)
	ps.isRunning = false

	// 关闭pubsub客户端
	if ps.pubsub != nil {
		err := ps.pubsub.Close()
		if err != nil {
			return fmt.Errorf("关闭发布订阅客户端失败: %w", err)
		}
		ps.pubsub = nil
	}

	// 清除状态
	ps.handlers = make(map[string][]MessageHandler)
	ps.patternHandlers = make(map[string][]MessageHandler)
	ps.activeChannels = make(map[string]struct{})

	log.Printf("Redis发布订阅服务已关闭")
	return nil
}

// startMessageLoop 启动消息接收处理循环
func (ps *RedisPubSub) startMessageLoop(ctx context.Context) {
	if ps.isRunning {
		return
	}

	ps.isRunning = true
	go func() {
		for {
			select {
			case <-ps.stopChan:
				return
			default:
				msg, err := ps.pubsub.ReceiveMessage(ctx)
				if err != nil {
					log.Printf("接收消息失败: %v", err)
					continue
				}

				ps.handleMessage(msg)
			}
		}
	}()
}

// handleMessage 处理接收到的消息
func (ps *RedisPubSub) handleMessage(msg *redis.Message) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// 处理普通频道的消息
	if handlers, ok := ps.handlers[msg.Channel]; ok {
		payload := []byte(msg.Payload)
		for _, handler := range handlers {
			go handler(msg.Channel, payload)
		}
	}

	// 处理模式匹配频道的消息
	if msg.Pattern != "" {
		if handlers, ok := ps.patternHandlers[msg.Pattern]; ok {
			payload := []byte(msg.Payload)
			for _, handler := range handlers {
				go handler(msg.Channel, payload)
			}
		}
	}
}
