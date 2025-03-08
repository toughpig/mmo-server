package gateway

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"

	"github.com/yourusername/mmo-server/pkg/protocol"
)

// SessionState 表示会话状态
type SessionState int

const (
	// SessionStateInitialized 初始化状态
	SessionStateInitialized SessionState = iota
	// SessionStateAuthenticated 已认证状态
	SessionStateAuthenticated
	// SessionStateDisconnected 已断开连接状态
	SessionStateDisconnected
)

// Session 客户端会话
type Session struct {
	ID             string
	UserID         string
	ConnectionType protocol.ConnectionType
	LastActivity   time.Time
	wsConn         *websocket.Conn
	secureConn     SecureConnection
	quicConn       quic.Connection
	quicStream     quic.Stream
	mu             sync.RWMutex
	closed         bool
}

// ErrSessionNotFound 会话未找到错误
var ErrSessionNotFound = errors.New("session not found")

// ErrSessionAlreadyExists 会话已存在错误
var ErrSessionAlreadyExists = errors.New("session already exists")

// SessionManager 管理所有客户端会话
type SessionManager struct {
	sessions      map[string]*Session
	mu            sync.RWMutex
	heartbeatTTL  time.Duration
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewSessionManager 创建新的会话管理器
func NewSessionManager(heartbeatTTL, checkInterval time.Duration) *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &SessionManager{
		sessions:      make(map[string]*Session),
		heartbeatTTL:  heartbeatTTL,
		checkInterval: checkInterval,
		ctx:           ctx,
		cancel:        cancel,
	}

	go manager.heartbeatChecker()

	return manager
}

// heartbeatChecker 定期检查会话心跳
func (sm *SessionManager) heartbeatChecker() {
	ticker := time.NewTicker(sm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.checkHeartbeats()
		}
	}
}

// checkHeartbeats 检查所有会话的心跳状态
func (sm *SessionManager) checkHeartbeats() {
	now := time.Now()
	var expiredSessions []string

	sm.mu.RLock()
	for id, session := range sm.sessions {
		if now.Sub(session.LastActivity) > sm.heartbeatTTL {
			expiredSessions = append(expiredSessions, id)
		}
	}
	sm.mu.RUnlock()

	// 移除过期会话
	for _, id := range expiredSessions {
		log.Printf("Session expired: %s", id)
		sm.RemoveSession(id)
	}
}

// NewSession 创建WebSocket会话
func NewSession(id string, connType protocol.ConnectionType, conn *websocket.Conn) *Session {
	return &Session{
		ID:             id,
		ConnectionType: connType,
		LastActivity:   time.Now(),
		wsConn:         conn,
	}
}

// NewSessionWithSecureConnection 创建安全WebSocket会话
func NewSessionWithSecureConnection(id string, connType protocol.ConnectionType, conn SecureConnection) *Session {
	return &Session{
		ID:             id,
		ConnectionType: connType,
		LastActivity:   time.Now(),
		secureConn:     conn,
	}
}

// NewSessionWithQUIC 创建QUIC会话
func NewSessionWithQUIC(id string, connType protocol.ConnectionType, conn quic.Connection) *Session {
	return &Session{
		ID:             id,
		ConnectionType: connType,
		LastActivity:   time.Now(),
		quicConn:       conn,
	}
}

// GetSession 获取指定ID的会话
func (sm *SessionManager) GetSession(sessionID string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// GetAllSessions 获取所有会话
func (sm *SessionManager) GetAllSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]*Session, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// RemoveSession 移除会话
func (sm *SessionManager) RemoveSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	// 关闭会话
	session.closed = true

	// 从映射中移除会话
	delete(sm.sessions, sessionID)
	log.Printf("Session removed: %s", sessionID)

	return nil
}

// GetSessionCount 获取当前会话数量
func (sm *SessionManager) GetSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return len(sm.sessions)
}

// Stop 停止会话管理器
func (sm *SessionManager) Stop() {
	sm.cancel()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 关闭所有会话
	for _, session := range sm.sessions {
		session.closed = true
	}

	sm.sessions = make(map[string]*Session)
	log.Println("Session manager stopped")
}

// SetUserID 设置用户ID
func (s *Session) SetUserID(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UserID = userID
}

// ReadMessage 读取消息
func (s *Session) ReadMessage() (int, []byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, nil, errors.New("session closed")
	}

	// 更新最后活动时间
	s.LastActivity = time.Now()

	// 根据连接类型读取消息
	switch s.ConnectionType {
	case protocol.ConnectionTypeWebSocket:
		if s.wsConn != nil {
			return s.wsConn.ReadMessage()
		} else if s.secureConn != nil {
			// 从安全连接接收数据
			data, err := s.secureConn.Receive()
			if err != nil {
				return 0, nil, err
			}
			return websocket.BinaryMessage, data, nil
		}
	case protocol.ConnectionTypeQUIC:
		if s.quicStream != nil {
			// 读取消息长度
			var msgLen uint32
			if err := binary.Read(s.quicStream, binary.BigEndian, &msgLen); err != nil {
				return 0, nil, err
			}

			// 读取消息类型
			var msgType uint8
			if err := binary.Read(s.quicStream, binary.BigEndian, &msgType); err != nil {
				return 0, nil, err
			}

			// 读取消息内容
			data := make([]byte, msgLen)
			if _, err := s.quicStream.Read(data); err != nil {
				return 0, nil, err
			}

			return int(msgType), data, nil
		}
	}

	return 0, nil, errors.New("no valid connection")
}

// WriteMessage 写入消息
func (s *Session) WriteMessage(messageType int, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("session closed")
	}

	// 更新最后活动时间
	s.LastActivity = time.Now()

	// 根据连接类型写入消息
	switch s.ConnectionType {
	case protocol.ConnectionTypeWebSocket:
		if s.wsConn != nil {
			return s.wsConn.WriteMessage(messageType, data)
		} else if s.secureConn != nil {
			// 通过安全连接发送数据
			return s.secureConn.Send(data)
		}
	case protocol.ConnectionTypeQUIC:
		if s.quicStream != nil {
			// 写入消息长度
			if err := binary.Write(s.quicStream, binary.BigEndian, uint32(len(data))); err != nil {
				return err
			}

			// 写入消息类型
			if err := binary.Write(s.quicStream, binary.BigEndian, uint8(messageType)); err != nil {
				return err
			}

			// 写入消息内容
			if _, err := s.quicStream.Write(data); err != nil {
				return err
			}

			return nil
		}
	}

	return errors.New("no valid connection")
}

// Close 关闭会话
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// 根据连接类型关闭连接
	switch s.ConnectionType {
	case protocol.ConnectionTypeWebSocket:
		if s.wsConn != nil {
			return s.wsConn.Close()
		} else if s.secureConn != nil {
			return s.secureConn.Close()
		}
	case protocol.ConnectionTypeQUIC:
		if s.quicStream != nil {
			s.quicStream.Close()
		}
		if s.quicConn != nil {
			return s.quicConn.CloseWithError(0, "session closed")
		}
	}

	return nil
}

// IsClosed 检查会话是否已关闭
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// ToJSON 将会话转换为JSON数据
func (s *Session) ToJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 创建一个不包含私有字段的结构体
	type SessionJSON struct {
		ID             string                  `json:"id"`
		UserID         string                  `json:"user_id,omitempty"`
		ConnectionType protocol.ConnectionType `json:"conn_type"`
		LastActivity   time.Time               `json:"last_activity"`
	}

	sessionJSON := SessionJSON{
		ID:             s.ID,
		UserID:         s.UserID,
		ConnectionType: s.ConnectionType,
		LastActivity:   s.LastActivity,
	}

	return json.Marshal(sessionJSON)
}
