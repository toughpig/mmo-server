package rpc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "mmo-server/proto_define"
)

// ClientConn 包装了grpc连接和服务客户端
type ClientConn struct {
	Conn             *grpc.ClientConn
	PlayerService    pb.PlayerServiceClient
	MessageService   pb.MessageServiceClient
	RPCService       pb.RPCServiceClient
	LastUsed         time.Time     // 最后使用时间，用于清理长期不用的连接
	Creating         bool          // 标记连接是否正在创建中
	creationComplete chan struct{} // 连接创建完成信号
}

// ConnManager 管理gRPC连接
type ConnManager struct {
	connections        map[string]*ClientConn
	mu                 sync.RWMutex
	dialTimeout        time.Duration     // 连接超时时间
	idleTimeout        time.Duration     // 空闲连接超时时间
	cleanupInterval    time.Duration     // 清理间隔
	stopCleanup        chan struct{}     // 停止清理信号
	defaultDialOptions []grpc.DialOption // 默认连接选项
}

// NewConnManager 创建一个新的连接管理器
func NewConnManager() *ConnManager {
	// 默认连接选项
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second, // 每10秒ping一次
			Timeout:             5 * time.Second,  // 5秒后超时
			PermitWithoutStream: true,             // 允许没有活跃流时发送ping
		}),
		grpc.WithDefaultCallOptions(
			grpc.WaitForReady(true), // 等待直到连接就绪
		),
	}

	m := &ConnManager{
		connections:        make(map[string]*ClientConn),
		dialTimeout:        10 * time.Second, // 连接超时为10秒
		idleTimeout:        10 * time.Minute, // 空闲连接超时为10分钟
		cleanupInterval:    5 * time.Minute,  // 每5分钟清理一次
		stopCleanup:        make(chan struct{}),
		defaultDialOptions: dialOpts,
	}

	// 启动定期清理空闲连接的goroutine
	go m.cleanupWorker()

	return m
}

// 定期清理长时间未使用的连接
func (m *ConnManager) cleanupWorker() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupIdleConnections()
		case <-m.stopCleanup:
			return
		}
	}
}

// 清理空闲连接
func (m *ConnManager) cleanupIdleConnections() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for endpoint, conn := range m.connections {
		// 如果连接闲置时间超过设定值，关闭并删除
		if now.Sub(conn.LastUsed) > m.idleTimeout {
			log.Printf("Closing idle connection to %s", endpoint)
			conn.Conn.Close()
			delete(m.connections, endpoint)
		}
	}
}

// GetConnection 获取或创建到指定端点的连接
func (m *ConnManager) GetConnection(endpoint string) (*ClientConn, error) {
	// 先用读锁检查连接是否存在
	m.mu.RLock()
	conn, ok := m.connections[endpoint]

	// 检查连接状态
	if ok {
		// 如果连接正在创建中，等待创建完成
		if conn.Creating {
			m.mu.RUnlock()
			select {
			case <-conn.creationComplete:
				m.mu.RLock()
				conn = m.connections[endpoint]
				m.mu.RUnlock()
				return conn, nil
			case <-time.After(m.dialTimeout):
				return nil, fmt.Errorf("timed out waiting for connection to be created")
			}
		}

		// 检查连接状态
		state := conn.Conn.GetState()
		if state != connectivity.Shutdown && state != connectivity.TransientFailure {
			// 更新最后使用时间
			conn.LastUsed = time.Now()
			m.mu.RUnlock()
			return conn, nil
		}

		// 连接已关闭或失败，释放读锁，准备重新创建
		m.mu.RUnlock()
	} else {
		m.mu.RUnlock()
	}

	// 如果连接不存在或已关闭，则创建新连接
	m.mu.Lock()
	defer m.mu.Unlock()

	// 再次检查，防止在获取写锁期间其他协程已创建
	conn, ok = m.connections[endpoint]
	if ok {
		if conn.Creating {
			// 等待连接创建完成
			unlock := sync.OnceFunc(func() { m.mu.Unlock() })
			defer unlock()

			select {
			case <-conn.creationComplete:
				unlock()
				return m.connections[endpoint], nil
			case <-time.After(m.dialTimeout):
				return nil, fmt.Errorf("timed out waiting for connection to be created")
			}
		}

		// 再次检查连接状态
		state := conn.Conn.GetState()
		if state != connectivity.Shutdown && state != connectivity.TransientFailure {
			conn.LastUsed = time.Now()
			return conn, nil
		}

		// 关闭之前失败的连接
		conn.Conn.Close()
		delete(m.connections, endpoint)
	}

	// 创建新连接
	creationComplete := make(chan struct{})
	newConn := &ClientConn{
		LastUsed:         time.Now(),
		Creating:         true,
		creationComplete: creationComplete,
	}
	m.connections[endpoint] = newConn

	// 获取连接选项
	dialOptions := m.defaultDialOptions

	// 创建连接上下文（带超时）
	ctx, cancel := context.WithTimeout(context.Background(), m.dialTimeout)
	defer cancel()

	// 创建gRPC连接（在锁外进行）
	m.mu.Unlock()
	grpcConn, err := grpc.DialContext(ctx, endpoint, dialOptions...)
	m.mu.Lock()

	// 处理连接错误
	if err != nil {
		delete(m.connections, endpoint)
		close(creationComplete)
		return nil, fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}

	// 创建成功，更新连接信息
	newConn.Conn = grpcConn
	newConn.PlayerService = pb.NewPlayerServiceClient(grpcConn)
	newConn.MessageService = pb.NewMessageServiceClient(grpcConn)
	newConn.RPCService = pb.NewRPCServiceClient(grpcConn)
	newConn.Creating = false
	close(creationComplete)

	return newConn, nil
}

// Close 关闭指定的连接
func (m *ConnManager) Close(endpoint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.connections[endpoint]; ok {
		err := conn.Conn.Close()
		delete(m.connections, endpoint)
		return err
	}
	return nil
}

// CloseAll 关闭所有连接
func (m *ConnManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 停止清理工作
	close(m.stopCleanup)

	// 关闭所有连接
	for endpoint, conn := range m.connections {
		conn.Conn.Close()
		delete(m.connections, endpoint)
	}
}

// 全局连接管理器，可在多个地方共享
var DefaultConnManager = NewConnManager()

// GetPlayerServiceClient 获取PlayerService客户端
func GetPlayerServiceClient(endpoint string) (pb.PlayerServiceClient, error) {
	conn, err := DefaultConnManager.GetConnection(endpoint)
	if err != nil {
		return nil, err
	}
	return conn.PlayerService, nil
}

// GetMessageServiceClient 获取MessageService客户端
func GetMessageServiceClient(endpoint string) (pb.MessageServiceClient, error) {
	conn, err := DefaultConnManager.GetConnection(endpoint)
	if err != nil {
		return nil, err
	}
	return conn.MessageService, nil
}

// GetRPCServiceClient 获取RPCService客户端
func GetRPCServiceClient(endpoint string) (pb.RPCServiceClient, error) {
	conn, err := DefaultConnManager.GetConnection(endpoint)
	if err != nil {
		return nil, err
	}
	return conn.RPCService, nil
}
