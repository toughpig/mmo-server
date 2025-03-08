package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// QUICConnectionHandler QUIC连接处理器
type QUICConnectionHandler func(conn quic.Connection)

// QUICServer QUIC服务器
type QUICServer struct {
	config       QUICConfig
	listener     *quic.Listener
	tlsConfig    *tls.Config
	handler      QUICConnectionHandler
	ctx          context.Context
	cancel       context.CancelFunc
	connCount    int
	connCountMux sync.Mutex
}

// NewQUICServer 创建QUIC服务器
func NewQUICServer(config QUICConfig) (*QUICServer, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("QUIC server is disabled")
	}

	// 创建TLS配置
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &QUICServer{
		config:    config,
		tlsConfig: tlsConfig,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

// SetConnectionHandler 设置连接处理器
func (s *QUICServer) SetConnectionHandler(handler QUICConnectionHandler) {
	s.handler = handler
}

// Start 启动QUIC服务器
func (s *QUICServer) Start() error {
	if s.handler == nil {
		return fmt.Errorf("connection handler not set")
	}

	// 加载证书
	cert, err := tls.LoadX509KeyPair(s.config.CertFile, s.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to load TLS certificates: %w", err)
	}
	s.tlsConfig.Certificates = []tls.Certificate{cert}

	// 创建QUIC监听器
	listener, err := quic.ListenAddr(s.config.Address, s.tlsConfig, &quic.Config{
		MaxIdleTimeout:     time.Duration(s.config.IdleTimeout) * time.Second,
		MaxIncomingStreams: int64(s.config.MaxStreams),
	})
	if err != nil {
		return fmt.Errorf("failed to create QUIC listener: %w", err)
	}
	s.listener = listener

	log.Printf("Starting QUIC server on %s", s.config.Address)

	// 接受连接
	go s.acceptConnections()

	return nil
}

// Stop 停止QUIC服务器
func (s *QUICServer) Stop() error {
	if s.listener == nil {
		return nil
	}

	s.cancel()
	return (*s.listener).Close()
}

// acceptConnections 接受QUIC连接
func (s *QUICServer) acceptConnections() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := (*s.listener).Accept(s.ctx)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					log.Printf("Temporary error accepting QUIC connection: %v", err)
					time.Sleep(100 * time.Millisecond)
					continue
				}

				// 非临时错误，例如监听器已关闭
				if s.ctx.Err() == nil {
					log.Printf("Error accepting QUIC connection: %v", err)
				}
				return
			}

			// 增加连接计数
			s.connCountMux.Lock()
			s.connCount++
			s.connCountMux.Unlock()

			// 处理连接
			go s.handleConnection(conn)
		}
	}
}

// handleConnection 处理QUIC连接
func (s *QUICServer) handleConnection(conn quic.Connection) {
	defer func() {
		conn.CloseWithError(0, "connection closed")

		// 减少连接计数
		s.connCountMux.Lock()
		s.connCount--
		s.connCountMux.Unlock()
	}()

	// 调用用户提供的处理器
	s.handler(conn)
}

// GetConnectionCount 获取当前连接数
func (s *QUICServer) GetConnectionCount() int {
	s.connCountMux.Lock()
	defer s.connCountMux.Unlock()
	return s.connCount
}
