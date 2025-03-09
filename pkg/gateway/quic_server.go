package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"sync"
	"time"

	"mmo-server/pkg/protocol"
	"mmo-server/pkg/security"

	"github.com/quic-go/quic-go"
)

// QUICConnectionHandler 定义QUIC连接处理器函数类型
// 当接收到新的QUIC连接时，此函数会被调用
type QUICConnectionHandler func(conn quic.Connection)

// QUICServerConfig 配置QUIC服务器
type QUICServerConfig struct {
	ListenAddr      string        // 监听地址
	CertFile        string        // TLS证书文件路径
	KeyFile         string        // TLS私钥文件路径
	IdleTimeout     time.Duration // 空闲连接超时
	MaxStreams      int           // 每个连接的最大流数量
	MaxStreamBuffer int           // 每个流的最大缓冲区大小
}

// QUICServer 实现QUIC协议服务器
// 负责监听和管理QUIC连接，支持多流并发通信
type QUICServer struct {
	config            QUICServerConfig           // 服务器配置
	listener          *quic.Listener             // QUIC监听器
	tlsConfig         *tls.Config                // TLS配置
	handler           QUICConnectionHandler      // 连接处理器
	ctx               context.Context            // 上下文，用于控制服务器生命周期
	cancel            context.CancelFunc         // 用于取消上下文的函数
	connCount         int                        // 当前连接数
	connCountMux      sync.Mutex                 // 保护连接计数的互斥锁
	cryptoManager     *security.CryptoManager    // 加密管理器，用于安全通信
	sessionManager    *SessionManager            // 会话管理器
	protocolConverter protocol.ProtocolConverter // 协议转换器
	messageHandler    MessageHandler             // 消息处理器
}

// NewQUICServer 创建新的QUIC服务器实例
// 参数:
// - config: QUIC服务器配置
// - cryptoManager: 加密管理器
// - sessionManager: 会话管理器
// - protocolConverter: 协议转换器
// - messageHandler: 消息处理器
func NewQUICServer(
	config QUICServerConfig,
	cryptoManager *security.CryptoManager,
	sessionManager *SessionManager,
	protocolConverter protocol.ProtocolConverter,
	messageHandler MessageHandler,
) (*QUICServer, error) {
	// 创建TLS配置
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("无法加载TLS证书: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"mmo-quic"},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 创建并返回QUIC服务器
	return &QUICServer{
		config:            config,
		tlsConfig:         tlsConfig,
		ctx:               ctx,
		cancel:            cancel,
		connCountMux:      sync.Mutex{},
		cryptoManager:     cryptoManager,
		sessionManager:    sessionManager,
		protocolConverter: protocolConverter,
		messageHandler:    messageHandler,
	}, nil
}

// SetConnectionHandler 设置连接处理器
// 如果不设置，将使用默认处理器
func (s *QUICServer) SetConnectionHandler(handler QUICConnectionHandler) {
	s.handler = handler
}

// Start 启动QUIC服务器
// 加载TLS证书，创建监听器，并开始接受连接
func (s *QUICServer) Start() error {
	// 创建QUIC监听器
	listener, err := quic.ListenAddr(s.config.ListenAddr, s.tlsConfig, nil)
	if err != nil {
		return fmt.Errorf("无法启动QUIC监听器: %w", err)
	}
	s.listener = listener

	// 记录服务器启动日志
	log.Printf("QUIC服务器已启动，监听地址: %s", s.config.ListenAddr)

	// 在后台循环接收连接
	go s.acceptConnections()

	return nil
}

// defaultConnectionHandler 默认的连接处理器实现
// 创建QUICStreamHandler实例处理连接中的流
func (s *QUICServer) defaultConnectionHandler(conn quic.Connection) {
	// 创建流处理器
	handler := NewQUICStreamHandler(
		conn,
		s.protocolConverter,
		s.messageHandler,
		s.cryptoManager,
		s.sessionManager,
		s.config.MaxStreamBuffer,
	)

	// 处理连接
	handler.Handle()
}

// Stop 停止QUIC服务器
// 关闭监听器并取消上下文
func (s *QUICServer) Stop() error {
	if s.listener == nil {
		return nil
	}

	// 取消上下文，通知所有goroutine退出
	s.cancel()
	// 关闭监听器
	return (*s.listener).Close()
}

// acceptConnections 持续接受传入的QUIC连接
func (s *QUICServer) acceptConnections() {
	for {
		// 检查服务器是否已关闭
		select {
		case <-s.ctx.Done():
			return
		default:
			// 继续接受连接
		}

		// 接受新连接
		conn, err := s.listener.Accept(s.ctx)
		if err != nil {
			// 检查服务器是否已关闭
			if s.ctx.Err() != nil {
				return
			}
			log.Printf("接受QUIC连接失败: %v", err)
			continue
		}

		// 增加连接计数
		s.connCountMux.Lock()
		s.connCount++
		s.connCountMux.Unlock()

		// 处理连接（可能由用户定义的处理器或默认处理器）
		go s.handleConnection(conn)
	}
}

// handleConnection 处理单个QUIC连接
func (s *QUICServer) handleConnection(conn quic.Connection) {
	// 确保连接最终会关闭，并减少计数
	defer func() {
		conn.CloseWithError(0, "connection closed")
		s.connCountMux.Lock()
		s.connCount--
		s.connCountMux.Unlock()
	}()

	// 创建并启动流处理器
	streamHandler := NewQUICStreamHandler(
		conn,
		s.protocolConverter,
		s.messageHandler,
		s.cryptoManager,
		s.sessionManager,
		s.config.MaxStreamBuffer,
	)

	// 处理连接中的所有流
	streamHandler.Handle()
}

// GetConnectionCount 返回当前活跃连接数
func (s *QUICServer) GetConnectionCount() int {
	s.connCountMux.Lock()
	defer s.connCountMux.Unlock()
	return s.connCount
}

// BroadcastMessage 向所有连接广播消息
// 注意：当前实现不支持直接广播到QUIC连接
// 需要使用会话管理器实现
func (s *QUICServer) BroadcastMessage(msg *protocol.Message) {
	// 由于QUIC连接处理器管理连接，无法直接广播
	// 实现从SessionManager获取所有会话，并通过会话发送消息的逻辑
	// 此方法留作将来实现
	log.Printf("QUIC BroadcastMessage not implemented yet")
}
