package gateway

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"

	"mmo-server/pkg/protocol"
	"mmo-server/pkg/security"
)

// GatewayHandler 网关处理器，负责处理来自不同协议的连接和消息
type GatewayHandler struct {
	config            *Config
	router            *protocol.MessageRouter
	converter         protocol.ProtocolConverter
	registry          *protocol.HandlerRegistry
	wsServer          *WebSocketServer
	secureWsServer    *SecureWebSocketServer
	quicServer        *QUICServer
	sessions          map[string]*Session
	sessionsByUser    map[string]map[string]*Session // userID -> map[sessionID]*Session
	sessionsMutex     sync.RWMutex
	shutdownCh        chan struct{}
	serviceDiscovery  protocol.ServiceDiscovery
	serviceRegistered bool
	cryptoManager     *security.CryptoManager    // 加密管理器
	sessionManager    *SessionManager            // 会话管理器
	protocolConverter protocol.ProtocolConverter // 协议转换器
}

// NewGatewayHandler 创建网关处理器
func NewGatewayHandler(config *Config) (*GatewayHandler, error) {
	// 创建服务发现
	serviceDiscovery := protocol.NewInMemoryServiceDiscovery()

	// 创建负载均衡器
	var loadBalancer protocol.LoadBalancer
	switch config.LoadBalancer.Type {
	case "round_robin":
		loadBalancer = protocol.NewRoundRobinLoadBalancer()
	case "weighted":
		loadBalancer = protocol.NewWeightedLoadBalancer()
	default:
		loadBalancer = protocol.NewRoundRobinLoadBalancer()
	}

	// 创建协议转换器
	converter := protocol.NewProtocolConverter()

	// 创建消息处理器注册表
	registry := protocol.NewHandlerRegistry()

	// 创建消息路由器
	router := protocol.NewMessageRouter(serviceDiscovery, loadBalancer, converter, registry)

	// 设置路由超时
	if config.Router.Timeout > 0 {
		router.SetTimeout(time.Duration(config.Router.Timeout) * time.Millisecond)
	}

	// 注册服务类型和服务名映射
	for serviceType, serviceName := range config.Router.ServiceMap {
		serviceTypeInt := protocol.ServiceType(serviceType)
		router.RegisterServiceType(serviceTypeInt, serviceName)
	}

	// 初始化处理器
	handler := &GatewayHandler{
		config:            config,
		router:            router,
		converter:         converter,
		registry:          registry,
		sessions:          make(map[string]*Session),
		sessionsByUser:    make(map[string]map[string]*Session),
		shutdownCh:        make(chan struct{}),
		serviceDiscovery:  serviceDiscovery,
		serviceRegistered: false,
		protocolConverter: converter, // 使用同一个转换器实例
	}

	// 创建加密管理器
	cryptoManager, err := security.NewCryptoManager(&security.CryptoConfig{
		KeyRotationInterval: 86400, // 24小时
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create crypto manager: %w", err)
	}
	handler.cryptoManager = cryptoManager

	// 创建会话管理器
	sessionManager := NewSessionManager(
		time.Duration(config.MaxSessionInactiveTime)*time.Second,
		time.Duration(config.SessionCleanupInterval)*time.Second,
	)
	handler.sessionManager = sessionManager

	// 注册内部消息处理器
	handler.registerInternalHandlers()

	return handler, nil
}

// Start 启动网关处理器
func (h *GatewayHandler) Start() error {
	var err error

	// 注册网关服务实例
	err = h.registerServiceInstance()
	if err != nil {
		return fmt.Errorf("failed to register gateway service: %w", err)
	}

	// 启动WebSocket服务器
	if h.config.WebSocket.Enabled {
		h.wsServer, err = NewWebSocketServer(h.config.WebSocket)
		if err != nil {
			return fmt.Errorf("failed to create WebSocket server: %w", err)
		}

		// 设置连接处理器
		h.wsServer.SetConnectionHandler(h.handleWebSocketConnection)

		// 启动WebSocket服务器
		go func() {
			if err := h.wsServer.Start(); err != nil {
				log.Printf("WebSocket server error: %v", err)
			}
		}()

		log.Printf("WebSocket server started on %s", h.config.WebSocket.Address)
	}

	// 启动安全WebSocket服务器
	if h.config.SecureWebSocket.Enabled {
		h.secureWsServer, err = NewSecureWebSocketServer(h.config.SecureWebSocket)
		if err != nil {
			return fmt.Errorf("failed to create secure WebSocket server: %w", err)
		}

		// 设置连接处理器
		h.secureWsServer.SetConnectionHandler(h.handleSecureWebSocketConnection)

		// 启动安全WebSocket服务器
		go func() {
			if err := h.secureWsServer.Start(); err != nil {
				log.Printf("Secure WebSocket server error: %v", err)
			}
		}()

		log.Printf("Secure WebSocket server started on %s", h.config.SecureWebSocket.Address)
	}

	// 启动QUIC服务器
	if h.config.QUIC.Enabled {
		// 创建QUIC服务器配置
		quicConfig := QUICServerConfig{
			ListenAddr:      h.config.QUIC.Address,
			CertFile:        h.config.QUIC.CertFile,
			KeyFile:         h.config.QUIC.KeyFile,
			IdleTimeout:     time.Duration(h.config.QUIC.IdleTimeout) * time.Second,
			MaxStreams:      h.config.QUIC.MaxStreams,
			MaxStreamBuffer: h.config.QUIC.BufferSize,
		}

		// 创建QUIC服务器
		h.quicServer, err = NewQUICServer(
			quicConfig,
			h.cryptoManager,
			h.sessionManager,
			h.protocolConverter,
			h,
		)
		if err != nil {
			return fmt.Errorf("failed to create QUIC server: %w", err)
		}

		// 设置连接处理器
		h.quicServer.SetConnectionHandler(h.handleQUICConnection)

		// 启动QUIC服务器
		go func() {
			if err := h.quicServer.Start(); err != nil {
				log.Printf("QUIC server error: %v", err)
			}
		}()

		log.Printf("QUIC server started on %s", h.config.QUIC.Address)
	}

	// 启动会话清理
	go h.sessionCleanupLoop()

	return nil
}

// Stop 停止网关处理器
func (h *GatewayHandler) Stop() error {
	// 发送关闭信号
	close(h.shutdownCh)

	// 停止WebSocket服务器
	if h.wsServer != nil {
		if err := h.wsServer.Stop(); err != nil {
			log.Printf("Error stopping WebSocket server: %v", err)
		}
	}

	// 停止安全WebSocket服务器
	if h.secureWsServer != nil {
		if err := h.secureWsServer.Stop(); err != nil {
			log.Printf("Error stopping secure WebSocket server: %v", err)
		}
	}

	// 停止QUIC服务器
	if h.quicServer != nil {
		if err := h.quicServer.Stop(); err != nil {
			log.Printf("Error stopping QUIC server: %v", err)
		}
	}

	// 关闭所有会话
	h.sessionsMutex.Lock()
	for _, session := range h.sessions {
		session.Close()
	}
	h.sessionsMutex.Unlock()

	// 注销服务实例
	if h.serviceRegistered {
		instanceID := fmt.Sprintf("gateway-%s", h.config.ID)
		if err := h.serviceDiscovery.DeregisterServiceInstance(instanceID); err != nil {
			log.Printf("Error deregistering gateway service: %v", err)
		}
	}

	return nil
}

// handleWebSocketConnection 处理WebSocket连接
func (h *GatewayHandler) handleWebSocketConnection(conn *websocket.Conn) {
	// 创建会话
	sessionID := uuid.New().String()
	session := NewSession(sessionID, protocol.ConnectionTypeWebSocket, conn)

	// 注册会话
	h.registerSession(session)

	// 处理会话消息
	go h.handleSessionMessages(session)

	log.Printf("New WebSocket connection established: %s", sessionID)
}

// handleSecureWebSocketConnection 处理安全WebSocket连接
func (h *GatewayHandler) handleSecureWebSocketConnection(secureConn *SecureWebSocketConnection) {
	// 创建会话
	sessionID := uuid.New().String()
	session := NewSessionWithSecureConnection(sessionID, protocol.ConnectionTypeWebSocket, secureConn)

	// 注册会话
	h.registerSession(session)

	// 处理会话消息
	go h.handleSessionMessages(session)

	log.Printf("New secure WebSocket connection established: %s", sessionID)
}

// handleQUICConnection 处理QUIC连接
func (h *GatewayHandler) handleQUICConnection(conn quic.Connection) {
	// 创建会话
	sessionID := uuid.New().String()
	session := NewSessionWithQUIC(sessionID, protocol.ConnectionTypeQUIC, conn)

	// 注册会话
	h.registerSession(session)

	// 处理会话消息
	go h.handleSessionMessages(session)

	log.Printf("New QUIC connection established: %s", sessionID)
}

// registerSession 注册会话
func (h *GatewayHandler) registerSession(session *Session) {
	h.sessionsMutex.Lock()
	defer h.sessionsMutex.Unlock()

	h.sessions[session.ID] = session

	// 如果会话有用户ID，也按用户ID注册
	if session.UserID != "" {
		if _, ok := h.sessionsByUser[session.UserID]; !ok {
			h.sessionsByUser[session.UserID] = make(map[string]*Session)
		}
		h.sessionsByUser[session.UserID][session.ID] = session
	}
}

// unregisterSession 注销会话
func (h *GatewayHandler) unregisterSession(sessionID string) {
	h.sessionsMutex.Lock()
	defer h.sessionsMutex.Unlock()

	session, ok := h.sessions[sessionID]
	if !ok {
		return
	}

	// 如果会话有用户ID，也按用户ID注销
	if session.UserID != "" {
		if userSessions, ok := h.sessionsByUser[session.UserID]; ok {
			delete(userSessions, sessionID)
			if len(userSessions) == 0 {
				delete(h.sessionsByUser, session.UserID)
			}
		}
	}

	delete(h.sessions, sessionID)
}

// handleSessionMessages 处理会话消息
func (h *GatewayHandler) handleSessionMessages(session *Session) {
	defer func() {
		// 注销会话
		h.unregisterSession(session.ID)
		// 关闭会话
		session.Close()
		log.Printf("Session closed: %s", session.ID)
	}()

	for {
		// 接收消息
		msgType, data, err := session.ReadMessage()
		if err != nil {
			log.Printf("Error reading message from session %s: %v", session.ID, err)
			return
		}

		// 处理消息
		err = h.handleMessage(session, msgType, data)
		if err != nil {
			log.Printf("Error handling message from session %s: %v", session.ID, err)
			// 发送错误响应
			errorMsg := fmt.Sprintf("Error: %v", err)
			if err := session.WriteMessage(msgType, []byte(errorMsg)); err != nil {
				log.Printf("Error sending error message to session %s: %v", session.ID, err)
				return
			}
		}
	}
}

// handleMessage 处理消息
func (h *GatewayHandler) handleMessage(session *Session, msgType int, data []byte) error {
	// 将消息转换为内部格式
	internalMsg, err := h.converter.ToInternal(session.ConnectionType, protocol.FormatType(msgType), data)
	if err != nil {
		return fmt.Errorf("failed to convert message to internal format: %w", err)
	}

	// 设置会话ID
	internalMsg.SessionID = session.ID

	// 如果消息没有源服务，设置为网关
	if internalMsg.SourceService == "" {
		internalMsg.SourceService = "gateway"
	}

	// 处理消息
	ctx := context.Background()
	response, err := h.router.Route(ctx, internalMsg)
	if err != nil {
		return fmt.Errorf("failed to route message: %w", err)
	}

	// 如果有响应，发送回客户端
	if response != nil {
		// 将内部消息转换回客户端格式
		responseData, err := h.converter.FromInternal(session.ConnectionType, protocol.FormatType(msgType), response)
		if err != nil {
			return fmt.Errorf("failed to convert response from internal format: %w", err)
		}

		// 发送响应
		if err := session.WriteMessage(msgType, responseData); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
	}

	return nil
}

// registerInternalHandlers 注册内部消息处理器
func (h *GatewayHandler) registerInternalHandlers() {
	// 注册心跳处理器
	h.registry.RegisterHandler(protocol.ServiceTypeGateway, 1, func(msg *protocol.Message) (*protocol.Message, error) {
		// 创建心跳响应
		response := &protocol.Message{
			Version:            protocol.ProtocolVersion,
			Flags:              protocol.FlagPong,
			ServiceType:        protocol.ServiceTypeGateway,
			MessageType:        1, // 心跳响应
			SequenceID:         msg.SequenceID,
			Timestamp:          uint64(time.Now().UnixMicro()),
			PayloadLength:      0,
			SourceService:      "gateway",
			DestinationService: msg.SourceService,
			CorrelationID:      msg.CorrelationID,
			SessionID:          msg.SessionID,
			Payload:            []byte("pong"),
		}
		return response, nil
	})

	// 注册会话信息处理器
	h.registry.RegisterHandler(protocol.ServiceTypeGateway, 2, func(msg *protocol.Message) (*protocol.Message, error) {
		// 获取会话信息
		h.sessionsMutex.RLock()
		sessionCount := len(h.sessions)
		userCount := len(h.sessionsByUser)
		h.sessionsMutex.RUnlock()

		// 创建响应
		payload := fmt.Sprintf(`{"sessions": %d, "users": %d}`, sessionCount, userCount)
		response := &protocol.Message{
			Version:            protocol.ProtocolVersion,
			Flags:              protocol.FlagNone,
			ServiceType:        protocol.ServiceTypeGateway,
			MessageType:        2, // 会话信息响应
			SequenceID:         msg.SequenceID,
			Timestamp:          uint64(time.Now().UnixMicro()),
			PayloadLength:      uint32(len(payload)),
			SourceService:      "gateway",
			DestinationService: msg.SourceService,
			CorrelationID:      msg.CorrelationID,
			SessionID:          msg.SessionID,
			Payload:            []byte(payload),
		}
		return response, nil
	})
}

// sessionCleanupLoop 会话清理循环
func (h *GatewayHandler) sessionCleanupLoop() {
	ticker := time.NewTicker(time.Duration(h.config.SessionCleanupInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.cleanupInactiveSessions()
		case <-h.shutdownCh:
			return
		}
	}
}

// cleanupInactiveSessions 清理不活跃的会话
func (h *GatewayHandler) cleanupInactiveSessions() {
	now := time.Now()
	maxInactiveTime := time.Duration(h.config.MaxSessionInactiveTime) * time.Second

	h.sessionsMutex.Lock()
	defer h.sessionsMutex.Unlock()

	for id, session := range h.sessions {
		if now.Sub(session.LastActivity) > maxInactiveTime {
			log.Printf("Closing inactive session: %s (inactive for %v)", id, now.Sub(session.LastActivity))

			// 如果会话有用户ID，也按用户ID注销
			if session.UserID != "" {
				if userSessions, ok := h.sessionsByUser[session.UserID]; ok {
					delete(userSessions, id)
					if len(userSessions) == 0 {
						delete(h.sessionsByUser, session.UserID)
					}
				}
			}

			// 从会话映射中删除
			delete(h.sessions, id)

			// 关闭会话
			session.Close()
		}
	}
}

// registerServiceInstance 注册网关服务实例
func (h *GatewayHandler) registerServiceInstance() error {
	instanceID := fmt.Sprintf("gateway-%s", h.config.ID)
	instance := protocol.ServiceInstance{
		ID:          instanceID,
		ServiceName: "gateway",
		Address:     h.config.WebSocket.Address,
		Weight:      100,
		Status:      "up",
		LastSeen:    time.Now(),
		Metadata: map[string]string{
			"version": "1.0.0",
			"region":  h.config.Region,
		},
	}

	err := h.serviceDiscovery.RegisterServiceInstance(instance)
	if err != nil {
		return err
	}

	h.serviceRegistered = true
	return nil
}

// GetSessionCount 获取会话数量
func (h *GatewayHandler) GetSessionCount() int {
	h.sessionsMutex.RLock()
	defer h.sessionsMutex.RUnlock()
	return len(h.sessions)
}

// GetUserCount 获取用户数量
func (h *GatewayHandler) GetUserCount() int {
	h.sessionsMutex.RLock()
	defer h.sessionsMutex.RUnlock()
	return len(h.sessionsByUser)
}

// GetSession 获取会话
func (h *GatewayHandler) GetSession(sessionID string) (*Session, bool) {
	h.sessionsMutex.RLock()
	defer h.sessionsMutex.RUnlock()
	session, ok := h.sessions[sessionID]
	return session, ok
}

// GetUserSessions 获取用户的所有会话
func (h *GatewayHandler) GetUserSessions(userID string) []*Session {
	h.sessionsMutex.RLock()
	defer h.sessionsMutex.RUnlock()

	userSessions, ok := h.sessionsByUser[userID]
	if !ok {
		return nil
	}

	sessions := make([]*Session, 0, len(userSessions))
	for _, session := range userSessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// BroadcastToUser 向用户的所有会话广播消息
func (h *GatewayHandler) BroadcastToUser(userID string, msgType int, data []byte) error {
	sessions := h.GetUserSessions(userID)
	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found for user %s", userID)
	}

	for _, session := range sessions {
		if err := session.WriteMessage(msgType, data); err != nil {
			log.Printf("Error broadcasting to session %s: %v", session.ID, err)
		}
	}

	return nil
}

// BroadcastToAll 向所有会话广播消息
func (h *GatewayHandler) BroadcastToAll(msgType int, data []byte) {
	h.sessionsMutex.RLock()
	sessions := make([]*Session, 0, len(h.sessions))
	for _, session := range h.sessions {
		sessions = append(sessions, session)
	}
	h.sessionsMutex.RUnlock()

	for _, session := range sessions {
		if err := session.WriteMessage(msgType, data); err != nil {
			log.Printf("Error broadcasting to session %s: %v", session.ID, err)
		}
	}
}
