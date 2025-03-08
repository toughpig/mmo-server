package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectionHandler WebSocket连接处理器
type ConnectionHandler func(conn *websocket.Conn)

// WebSocketServer WebSocket服务器
type WebSocketServer struct {
	config     WebSocketConfig
	server     *http.Server
	upgrader   websocket.Upgrader
	handler    ConnectionHandler
	shutdownCh chan struct{}
}

// NewWebSocketServer 创建WebSocket服务器
func NewWebSocketServer(config WebSocketConfig) (*WebSocketServer, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("WebSocket server is disabled")
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  config.BufferSize,
		WriteBufferSize: config.BufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许所有来源
		},
	}

	return &WebSocketServer{
		config:     config,
		upgrader:   upgrader,
		shutdownCh: make(chan struct{}),
	}, nil
}

// SetConnectionHandler 设置连接处理器
func (s *WebSocketServer) SetConnectionHandler(handler ConnectionHandler) {
	s.handler = handler
}

// Start 启动WebSocket服务器
func (s *WebSocketServer) Start() error {
	if s.handler == nil {
		return fmt.Errorf("connection handler not set")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	s.server = &http.Server{
		Addr:         s.config.Address,
		Handler:      mux,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(s.config.IdleTimeout) * time.Second,
	}

	log.Printf("Starting WebSocket server on %s", s.config.Address)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// Stop 停止WebSocket服务器
func (s *WebSocketServer) Stop() error {
	if s.server == nil {
		return nil
	}

	close(s.shutdownCh)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// handleWebSocket 处理WebSocket连接
func (s *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// 设置读写超时
	conn.SetReadDeadline(time.Now().Add(time.Duration(s.config.ReadTimeout) * time.Second))
	conn.SetWriteDeadline(time.Now().Add(time.Duration(s.config.WriteTimeout) * time.Second))

	// 处理连接
	go s.handler(conn)
}
