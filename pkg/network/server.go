package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yourusername/mmo-server/pkg/config"
)

// Server 表示游戏服务器
type Server struct {
	config      *config.Config
	listener    net.Listener
	clients     map[string]*Client
	clientsLock sync.RWMutex
	handlers    map[int32]MessageHandler
	upgrader    websocket.Upgrader
	ctx         context.Context
	cancel      context.CancelFunc
}

// Client 表示连接的客户端
type Client struct {
	ID         string
	Conn       *websocket.Conn
	Server     *Server
	Send       chan []byte
	ctx        context.Context
	cancel     context.CancelFunc
	closeMutex sync.Mutex
	closed     bool
	lastPing   time.Time
}

// MessageHandler 处理特定类型消息的函数
type MessageHandler func(client *Client, message []byte) error

// NewServer 创建新的服务器实例
func NewServer(cfg *config.Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Server{
		config:   cfg,
		clients:  make(map[string]*Client),
		handlers: make(map[int32]MessageHandler),
		ctx:      ctx,
		cancel:   cancel,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有跨域请求，生产环境应当限制
			},
		},
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	
	s.listener = listener
	log.Printf("Server started on %s", addr)
	
	// 启动心跳检测
	go s.heartbeatChecker()
	
	// 主服务循环
	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Error accepting connection: %v", err)
				continue
			}
			
			// 创建客户端并启动处理
			client := s.newClient(conn)
			go client.readPump()
			go client.writePump()
		}
	}
}

// Stop 停止服务器
func (s *Server) Stop() {
	s.cancel()
	if s.listener != nil {
		s.listener.Close()
	}
	
	// 关闭所有客户端连接
	s.clientsLock.Lock()
	for _, client := range s.clients {
		client.Close()
	}
	s.clientsLock.Unlock()
	
	log.Println("Server stopped")
}

// RegisterHandler 注册消息处理器
func (s *Server) RegisterHandler(msgID int32, handler MessageHandler) {
	s.handlers[msgID] = handler
}

// Broadcast 向所有客户端广播消息
func (s *Server) Broadcast(message []byte) {
	s.clientsLock.RLock()
	defer s.clientsLock.RUnlock()
	
	for _, client := range s.clients {
		select {
		case client.Send <- message:
		default:
			client.Close()
		}
	}
}

// heartbeatChecker 定期检查客户端心跳
func (s *Server) heartbeatChecker() {
	ticker := time.NewTicker(time.Duration(s.config.Server.HeartbeatIntervalSec) * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkHeartbeats()
		}
	}
}

// checkHeartbeats 检查所有客户端的心跳状态
func (s *Server) checkHeartbeats() {
	timeout := time.Duration(s.config.Server.HeartbeatIntervalSec*2) * time.Second
	now := time.Now()
	
	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()
	
	for id, client := range s.clients {
		if now.Sub(client.lastPing) > timeout {
			log.Printf("Client %s timed out", id)
			client.Close()
			delete(s.clients, id)
		}
	}
}

// newClient 创建新的客户端
func (s *Server) newClient(conn net.Conn) *Client {
	ctx, cancel := context.WithCancel(s.ctx)
	clientID := generateID() // 生成唯一ID的函数
	
	// 使用websocket升级连接
	wsConn, err := s.upgrader.Upgrade(conn, nil, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		conn.Close()
		return nil
	}
	
	client := &Client{
		ID:       clientID,
		Conn:     wsConn,
		Server:   s,
		Send:     make(chan []byte, 256),
		ctx:      ctx,
		cancel:   cancel,
		lastPing: time.Now(),
	}
	
	// 添加到客户端列表
	s.clientsLock.Lock()
	s.clients[clientID] = client
	s.clientsLock.Unlock()
	
	log.Printf("New client connected: %s", clientID)
	return client
}

// readPump 处理客户端读取循环
func (c *Client) readPump() {
	defer func() {
		c.Close()
	}()
	
	// 设置读取超时
	c.Conn.SetReadDeadline(time.Now().Add(time.Duration(c.Server.config.Server.ReadTimeoutMs) * time.Millisecond))
	c.Conn.SetPongHandler(func(string) error {
		c.lastPing = time.Now()
		c.Conn.SetReadDeadline(time.Now().Add(time.Duration(c.Server.config.Server.ReadTimeoutMs) * time.Millisecond))
		return nil
	})
	
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error reading message: %v", err)
			}
			break
		}
		
		// 处理接收到的消息
		c.handleMessage(message)
	}
}

// writePump 处理客户端写入循环
func (c *Client) writePump() {
	ticker := time.NewTicker(time.Duration(c.Server.config.Server.HeartbeatIntervalSec/2) * time.Second)
	defer func() {
		ticker.Stop()
		c.Close()
	}()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(time.Duration(c.Server.config.Server.WriteTimeoutMs) * time.Millisecond))
			if !ok {
				// 通道已关闭
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			w, err := c.Conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}
			w.Write(message)
			
			// 添加队列中的其他消息
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write(<-c.Send)
			}
			
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(time.Duration(c.Server.config.Server.WriteTimeoutMs) * time.Millisecond))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Close 关闭客户端连接
func (c *Client) Close() {
	c.closeMutex.Lock()
	defer c.closeMutex.Unlock()
	
	if c.closed {
		return
	}
	
	c.closed = true
	c.cancel()
	c.Conn.Close()
	
	// 从服务器的客户端列表中删除
	c.Server.clientsLock.Lock()
	delete(c.Server.clients, c.ID)
	c.Server.clientsLock.Unlock()
	
	log.Printf("Client disconnected: %s", c.ID)
}

// Send 向客户端发送消息
func (c *Client) SendMessage(message []byte) error {
	select {
	case c.Send <- message:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// handleMessage 处理接收到的消息
func (c *Client) handleMessage(message []byte) {
	// TODO: 从消息中解析消息ID
	// 这里简化处理，假设前4字节是消息ID
	if len(message) < 4 {
		log.Printf("Message too short")
		return
	}
	
	// 将前4个字节作为消息ID (大端序)
	msgID := int32(message[0])<<24 | int32(message[1])<<16 | int32(message[2])<<8 | int32(message[3])
	
	// 查找并调用对应的处理器
	handler, ok := c.Server.handlers[msgID]
	if !ok {
		log.Printf("No handler for message ID %d", msgID)
		return
	}
	
	if err := handler(c, message); err != nil {
		log.Printf("Error handling message %d: %v", msgID, err)
	}
}

// generateID 生成唯一ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
} 