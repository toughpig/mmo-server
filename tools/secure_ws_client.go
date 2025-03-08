package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

// 消息类型常量
const (
	MessageTypeHandshake byte = 0x01
	MessageTypeData      byte = 0x02
	MessageTypePing      byte = 0x03
	MessageTypePong      byte = 0x04
	MessageTypeClose     byte = 0x05

	HandshakeRequest  byte = 0x01
	HandshakeResponse byte = 0x02
)

// HandshakeMessage 握手消息
type HandshakeMessage struct {
	Type      byte   `json:"type"`
	PublicKey []byte `json:"public_key"`
	SessionID string `json:"session_id"`
	Version   string `json:"version"`
}

// EncryptedMessage 加密消息
type EncryptedMessage struct {
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
	SessionID  string `json:"session_id"`
}

// SecureClient 安全客户端
type SecureClient struct {
	conn       *websocket.Conn
	privateKey *ecdsa.PrivateKey
	publicKey  []byte
	sharedKey  []byte
	sessionID  string
	isSecure   bool
}

// NewSecureClient 创建新的安全客户端
func NewSecureClient() (*SecureClient, error) {
	// 生成ECDH密钥对
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ECDH key pair: %w", err)
	}

	// 序列化公钥
	publicKey := elliptic.Marshal(privateKey.PublicKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y)

	return &SecureClient{
		privateKey: privateKey,
		publicKey:  publicKey,
		sessionID:  fmt.Sprintf("client-%d", time.Now().UnixNano()),
		isSecure:   false,
	}, nil
}

// Connect 连接到服务器
func (sc *SecureClient) Connect(url string) error {
	// 连接WebSocket服务器
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("connect error: %w", err)
	}
	sc.conn = conn
	return nil
}

// Close 关闭连接
func (sc *SecureClient) Close() error {
	if sc.conn != nil {
		// 如果是安全连接，发送关闭消息
		if sc.isSecure {
			closeMsg := []byte{MessageTypeClose, 0, 0, 0, 0}
			binary.BigEndian.PutUint32(closeMsg[1:5], 0)
			sc.conn.WriteMessage(websocket.BinaryMessage, closeMsg)
		}
		return sc.conn.Close()
	}
	return nil
}

// Handshake 进行加密握手
func (sc *SecureClient) Handshake() error {
	if sc.isSecure {
		return nil
	}

	// 创建握手请求
	hsRequest := HandshakeMessage{
		Type:      HandshakeRequest,
		PublicKey: sc.publicKey,
		SessionID: sc.sessionID,
		Version:   "1.0",
	}

	// 序列化握手请求
	hsRequestBytes, err := json.Marshal(hsRequest)
	if err != nil {
		return fmt.Errorf("marshal handshake request error: %w", err)
	}

	// 构建请求消息: 消息类型(1) + 握手类型(1) + 握手消息长度(4) + 握手消息
	requestLen := uint32(len(hsRequestBytes))
	request := make([]byte, 6+requestLen)
	request[0] = MessageTypeHandshake
	request[1] = HandshakeRequest
	binary.BigEndian.PutUint32(request[2:6], requestLen)
	copy(request[6:], hsRequestBytes)

	// 发送握手请求
	if err := sc.conn.WriteMessage(websocket.BinaryMessage, request); err != nil {
		return fmt.Errorf("write handshake request error: %w", err)
	}

	log.Println("Handshake request sent")

	// 接收握手响应
	_, response, err := sc.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read handshake response error: %w", err)
	}

	// 解析握手响应
	if len(response) < 6 {
		return fmt.Errorf("handshake response too short")
	}

	msgType := response[0]
	handshakeType := response[1]
	handshakeMsgLen := binary.BigEndian.Uint32(response[2:6])

	if msgType != MessageTypeHandshake || handshakeType != HandshakeResponse {
		return fmt.Errorf("unexpected message type in handshake response: %d", msgType)
	}

	if len(response) < int(6+handshakeMsgLen) {
		return fmt.Errorf("handshake response content too short")
	}

	// 解析握手响应消息
	var hsResponse HandshakeMessage
	if err := json.Unmarshal(response[6:6+handshakeMsgLen], &hsResponse); err != nil {
		return fmt.Errorf("unmarshal handshake response error: %w", err)
	}

	// 验证会话ID
	if hsResponse.SessionID != sc.sessionID {
		return fmt.Errorf("session ID mismatch in handshake response: expected %s, got %s", sc.sessionID, hsResponse.SessionID)
	}

	// 从服务器公钥生成共享密钥
	serverPubKey := hsResponse.PublicKey
	x, y := elliptic.Unmarshal(elliptic.P256(), serverPubKey)
	if x == nil {
		return fmt.Errorf("invalid server public key")
	}

	serverPublicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	// 计算共享密钥
	sharedSecret, _ := serverPublicKey.Curve.ScalarMult(serverPublicKey.X, serverPublicKey.Y, sc.privateKey.D.Bytes())
	if sharedSecret == nil {
		return fmt.Errorf("failed to compute shared secret")
	}

	// 使用SHA-256派生密钥
	key := sha256.Sum256(sharedSecret.Bytes())
	sc.sharedKey = key[:]

	sc.isSecure = true
	log.Println("Secure handshake completed successfully")

	return nil
}

// Send 发送加密消息
func (sc *SecureClient) Send(data []byte) error {
	if !sc.isSecure {
		return fmt.Errorf("connection not secure, handshake required")
	}

	// 这里简化处理，实际应用需要实现AES-GCM加密
	// 在实际应用中，应该像服务端一样使用相同的加密算法
	// 这里我们只发送明文作为演示
	msgLen := uint32(len(data))
	message := make([]byte, 5+msgLen)
	message[0] = MessageTypeData
	binary.BigEndian.PutUint32(message[1:5], msgLen)
	copy(message[5:], data)

	return sc.conn.WriteMessage(websocket.BinaryMessage, message)
}

// Receive 接收并解密消息
func (sc *SecureClient) Receive() ([]byte, error) {
	if !sc.isSecure {
		return nil, fmt.Errorf("connection not secure, handshake required")
	}

	// 接收消息
	_, message, err := sc.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read message error: %w", err)
	}

	// 消息格式: 消息类型(1) + 消息长度(4) + 消息内容
	if len(message) < 5 {
		return nil, fmt.Errorf("message too short")
	}

	msgType := message[0]
	msgLen := binary.BigEndian.Uint32(message[1:5])

	if len(message) < int(5+msgLen) {
		return nil, fmt.Errorf("message content too short")
	}

	// 处理不同类型的消息
	switch msgType {
	case MessageTypeData:
		// 在实际应用中，这里需要解密消息
		// 由于这是一个简化的演示，我们直接返回消息内容
		return message[5 : 5+msgLen], nil
	case MessageTypePing:
		// 接收到ping，发送pong
		pong := []byte{MessageTypePong, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(pong[1:5], 0)
		if err := sc.conn.WriteMessage(websocket.BinaryMessage, pong); err != nil {
			return nil, fmt.Errorf("write pong error: %w", err)
		}
		// 递归调用继续接收有效数据
		return sc.Receive()
	case MessageTypeClose:
		return nil, fmt.Errorf("connection closed by server")
	default:
		return nil, fmt.Errorf("unknown message type: %d", msgType)
	}
}

func main() {
	// 解析命令行参数
	addr := flag.String("addr", "localhost:8081", "服务器地址")
	path := flag.String("path", "/ws", "WebSocket路径")
	messages := flag.Int("n", 5, "发送的消息数量")
	interval := flag.Int("i", 1000, "消息间隔(毫秒)")
	flag.Parse()

	// 创建安全客户端
	client, err := NewSecureClient()
	if err != nil {
		log.Fatalf("Failed to create secure client: %v", err)
	}
	defer client.Close()

	// 连接到服务器
	url := fmt.Sprintf("ws://%s%s", *addr, *path)
	log.Printf("Connecting to %s", url)
	if err := client.Connect(url); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// 进行加密握手
	log.Println("Performing secure handshake")
	if err := client.Handshake(); err != nil {
		log.Fatalf("Handshake failed: %v", err)
	}

	// 设置中断处理
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// 发送/接收通道
	done := make(chan struct{})

	// 接收协程
	go func() {
		defer close(done)
		for {
			message, err := client.Receive()
			if err != nil {
				log.Printf("Receive error: %v", err)
				return
			}
			log.Printf("Received: %s", message)
		}
	}()

	// 发送消息
	for i := 0; i < *messages; i++ {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Println("Interrupted, closing connection")
			client.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		default:
			message := fmt.Sprintf("Secure echo test message #%d", i+1)
			log.Printf("Sending: %s", message)
			if err := client.Send([]byte(message)); err != nil {
				log.Printf("Send error: %v", err)
				return
			}
			time.Sleep(time.Duration(*interval) * time.Millisecond)
		}
	}

	// 等待一会儿，确保所有响应都收到
	time.Sleep(time.Second)

	// 关闭连接
	log.Println("Test completed, closing connection")
	client.Close()
	<-done
}
