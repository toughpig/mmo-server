package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/toughpig/mmo-server/pkg/security"
)

// SecureConnectionHandler 安全连接处理器
type SecureConnectionHandler func(conn *SecureWebSocketConnection)

// SecureWebSocketServer 安全WebSocket服务器
type SecureWebSocketServer struct {
	config        SecureWSConfig
	server        *http.Server
	upgrader      websocket.Upgrader
	handler       SecureConnectionHandler
	cryptoManager *security.CryptoManager
	shutdownCh    chan struct{}
}

// NewSecureWebSocketServer 创建安全WebSocket服务器
func NewSecureWebSocketServer(config SecureWSConfig) (*SecureWebSocketServer, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("Secure WebSocket server is disabled")
	}

	// 创建加密管理器，使用默认的24小时轮换间隔
	cryptoManager, err := security.NewCryptoManager(24 * time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to create crypto manager: %w", err)
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  config.BufferSize,
		WriteBufferSize: config.BufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许所有来源
		},
	}

	return &SecureWebSocketServer{
		config:        config,
		upgrader:      upgrader,
		cryptoManager: cryptoManager,
		shutdownCh:    make(chan struct{}),
	}, nil
}

// SetConnectionHandler 设置连接处理器
func (s *SecureWebSocketServer) SetConnectionHandler(handler SecureConnectionHandler) {
	s.handler = handler
}

// Start 启动安全WebSocket服务器
func (s *SecureWebSocketServer) Start() error {
	if s.handler == nil {
		return fmt.Errorf("connection handler not set")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	// 配置TLS
	tlsConfig := &tls.Config{}

	s.server = &http.Server{
		Addr:         s.config.Address,
		Handler:      mux,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(s.config.IdleTimeout) * time.Second,
		TLSConfig:    tlsConfig,
	}

	log.Printf("Starting Secure WebSocket server on %s", s.config.Address)
	if err := s.server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// Stop 停止安全WebSocket服务器
func (s *SecureWebSocketServer) Stop() error {
	if s.server == nil {
		return nil
	}

	close(s.shutdownCh)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// handleWebSocket 处理WebSocket连接
func (s *SecureWebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Secure WebSocket upgrade error: %v", err)
		return
	}

	// 设置读写超时
	conn.SetReadDeadline(time.Now().Add(time.Duration(s.config.ReadTimeout) * time.Second))
	conn.SetWriteDeadline(time.Now().Add(time.Duration(s.config.WriteTimeout) * time.Second))

	// 创建安全连接
	sessionID := generateSessionID()
	secureConn := NewSecureWebSocketConnection(conn, s.cryptoManager, sessionID)

	// 执行握手
	if err := secureConn.Handshake(); err != nil {
		log.Printf("Secure WebSocket handshake error: %v", err)
		conn.Close()
		return
	}

	// 处理连接
	go s.handler(secureConn)
}

// generateSessionID 生成会话ID
func generateSessionID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
