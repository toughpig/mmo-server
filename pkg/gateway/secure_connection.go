package gateway

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"mmo-server/pkg/security"

	"github.com/gorilla/websocket"
)

// MessageType 消息类型
const (
	MessageTypeHandshake byte = 0x01
	MessageTypeData      byte = 0x02
	MessageTypePing      byte = 0x03
	MessageTypePong      byte = 0x04
	MessageTypeClose     byte = 0x05

	MessageTypeHandshakeRequest  byte = 0x01
	MessageTypeHandshakeResponse byte = 0x02
)

// HandshakeMessage 握手消息
type HandshakeMessage struct {
	Type      int    `json:"type"`
	PublicKey string `json:"public_key,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Timestamp int64  `json:"timestamp"`
	TTL       int    `json:"ttl"`
	Nonce     string `json:"nonce,omitempty"`
}

// EncryptedMessage 加密消息
type EncryptedMessage struct {
	Type      int    `json:"type"`
	Data      string `json:"data"`
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
	TTL       int    `json:"ttl"`
}

// SecureConnection 安全连接接口
type SecureConnection interface {
	// GetSessionID 获取会话ID
	GetSessionID() string
	// Send 发送数据
	Send(data []byte) error
	// Receive 接收数据
	Receive() ([]byte, error)
	// Close 关闭连接
	Close() error
	// IsEncrypted 连接是否已加密
	IsEncrypted() bool
	// Handshake 进行加密握手
	Handshake() error
}

// SecureWebSocketConnection WebSocket安全连接
type SecureWebSocketConnection struct {
	conn          *websocket.Conn
	sessionID     string
	cryptoManager *security.CryptoManager
	isEncrypted   bool
	sendMutex     sync.Mutex
	handshakeDone bool
}

// NewSecureWebSocketConnection 创建新的WebSocket安全连接
func NewSecureWebSocketConnection(conn *websocket.Conn, cryptoManager *security.CryptoManager, sessionID string) *SecureWebSocketConnection {
	return &SecureWebSocketConnection{
		conn:          conn,
		sessionID:     sessionID,
		cryptoManager: cryptoManager,
		isEncrypted:   false,
		handshakeDone: false,
	}
}

// GetSessionID 获取会话ID
func (sc *SecureWebSocketConnection) GetSessionID() string {
	return sc.sessionID
}

// IsEncrypted 连接是否已加密
func (sc *SecureWebSocketConnection) IsEncrypted() bool {
	return sc.isEncrypted
}

// Handshake 进行加密握手
func (sc *SecureWebSocketConnection) Handshake() error {
	if sc.handshakeDone {
		return nil
	}

	// 作为服务端，等待客户端发起握手请求
	_, message, err := sc.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read handshake request error: %w", err)
	}

	// 消息格式: 消息类型(1) + 握手类型(1) + 握手消息长度(4) + 握手消息
	if len(message) < 6 {
		return errors.New("invalid handshake message format")
	}

	msgType := message[0]
	handshakeType := message[1]
	handshakeMsgLen := binary.BigEndian.Uint32(message[2:6])

	if msgType != MessageTypeHandshake || handshakeType != MessageTypeHandshakeRequest {
		return errors.New("unexpected message type during handshake")
	}

	if len(message) < int(6+handshakeMsgLen) {
		return errors.New("handshake message too short")
	}

	// 解析握手请求
	var hsRequest HandshakeMessage
	if err := json.Unmarshal(message[6:6+handshakeMsgLen], &hsRequest); err != nil {
		return fmt.Errorf("unmarshal handshake request error: %w", err)
	}

	// 验证会话ID
	if hsRequest.SessionID != sc.sessionID {
		return fmt.Errorf("session ID mismatch: expected %s, got %s", sc.sessionID, hsRequest.SessionID)
	}

	// 从客户端公钥生成共享密钥
	_, err = sc.cryptoManager.DeriveSharedKey(sc.sessionID, []byte(hsRequest.PublicKey))
	if err != nil {
		return fmt.Errorf("derive shared key error: %w", err)
	}

	// 创建握手响应
	hsResponse := HandshakeMessage{
		Type:      int(MessageTypeHandshakeResponse),
		PublicKey: base64.StdEncoding.EncodeToString(sc.cryptoManager.GetPublicKeyBytes()),
		SessionID: sc.sessionID,
		Timestamp: time.Now().Unix(),
		TTL:       60,
		Nonce:     generateNonce(),
	}

	// 序列化握手响应
	hsResponseBytes, err := json.Marshal(hsResponse)
	if err != nil {
		return fmt.Errorf("marshal handshake response error: %w", err)
	}

	// 构建响应消息: 消息类型(1) + 握手类型(1) + 握手消息长度(4) + 握手消息
	responseLen := uint32(len(hsResponseBytes))
	response := make([]byte, 6+responseLen)
	response[0] = MessageTypeHandshake
	response[1] = MessageTypeHandshakeResponse
	binary.BigEndian.PutUint32(response[2:6], responseLen)
	copy(response[6:], hsResponseBytes)

	// 发送握手响应
	sc.sendMutex.Lock()
	err = sc.conn.WriteMessage(websocket.BinaryMessage, response)
	sc.sendMutex.Unlock()
	if err != nil {
		return fmt.Errorf("write handshake response error: %w", err)
	}

	// 握手完成，设置加密标志
	sc.isEncrypted = true
	sc.handshakeDone = true
	log.Printf("Secure WebSocket handshake completed for session %s", sc.sessionID)

	return nil
}

// Send 发送加密数据
func (sc *SecureWebSocketConnection) Send(data []byte) error {
	if !sc.isEncrypted {
		return errors.New("connection not encrypted, handshake required")
	}

	// 加密数据
	em, err := sc.cryptoManager.EncryptWithTimestamp(data, sc.sessionID, 60)
	if err != nil {
		return fmt.Errorf("encrypt error: %w", err)
	}

	// 序列化加密消息
	emBytes, err := json.Marshal(em)
	if err != nil {
		return fmt.Errorf("marshal encrypted message error: %w", err)
	}

	// 构建消息: 消息类型(1) + 消息长度(4) + 加密消息
	msgLen := uint32(len(emBytes))
	message := make([]byte, 5+msgLen)
	message[0] = MessageTypeData
	binary.BigEndian.PutUint32(message[1:5], msgLen)
	copy(message[5:], emBytes)

	// 发送消息
	sc.sendMutex.Lock()
	err = sc.conn.WriteMessage(websocket.BinaryMessage, message)
	sc.sendMutex.Unlock()
	if err != nil {
		return fmt.Errorf("write message error: %w", err)
	}

	return nil
}

// Receive 接收并解密数据
func (sc *SecureWebSocketConnection) Receive() ([]byte, error) {
	if !sc.isEncrypted {
		return nil, errors.New("connection not encrypted, handshake required")
	}

	// 接收消息
	_, message, err := sc.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read message error: %w", err)
	}

	// 消息格式: 消息类型(1) + 消息长度(4) + 消息内容
	if len(message) < 5 {
		return nil, errors.New("message too short")
	}

	msgType := message[0]
	msgLen := binary.BigEndian.Uint32(message[1:5])

	if len(message) < int(5+msgLen) {
		return nil, errors.New("message content too short")
	}

	// 处理不同类型的消息
	switch msgType {
	case MessageTypeData:
		// 解析加密消息
		var em security.EncryptedMessage
		if err := json.Unmarshal(message[5:5+msgLen], &em); err != nil {
			return nil, fmt.Errorf("unmarshal encrypted message error: %w", err)
		}

		// 解密消息
		data, err := sc.cryptoManager.DecryptWithTimestamp(&em)
		if err != nil {
			return nil, fmt.Errorf("decrypt error: %w", err)
		}

		return data, nil

	case MessageTypePing:
		// 接收到ping，发送pong
		pong := []byte{MessageTypePong, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(pong[1:5], 0)
		sc.sendMutex.Lock()
		err := sc.conn.WriteMessage(websocket.BinaryMessage, pong)
		sc.sendMutex.Unlock()
		if err != nil {
			return nil, fmt.Errorf("write pong message error: %w", err)
		}
		// 递归调用继续接收有效数据
		return sc.Receive()

	case MessageTypeClose:
		return nil, io.EOF

	default:
		return nil, fmt.Errorf("unknown message type: %d", msgType)
	}
}

// Close 关闭连接
func (sc *SecureWebSocketConnection) Close() error {
	// 发送关闭消息
	if sc.isEncrypted {
		closeMsg := []byte{MessageTypeClose, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(closeMsg[1:5], 0)
		sc.sendMutex.Lock()
		sc.conn.WriteMessage(websocket.BinaryMessage, closeMsg)
		sc.sendMutex.Unlock()
	}

	// 从加密管理器中删除会话密钥
	sc.cryptoManager.RemoveSessionKey(sc.sessionID)

	// 关闭WebSocket连接
	return sc.conn.Close()
}

// generateNonce 生成随机nonce
func generateNonce() string {
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Printf("Error generating nonce: %v", err)
		// 使用时间戳作为备用
		binary.BigEndian.PutUint64(nonce, uint64(time.Now().UnixNano()))
	}
	return base64.StdEncoding.EncodeToString(nonce)
}
