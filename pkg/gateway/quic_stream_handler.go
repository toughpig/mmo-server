package gateway

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"mmo-server/pkg/protocol"
	"mmo-server/pkg/security"
	"mmo-server/pkg/utils"

	"github.com/quic-go/quic-go"
)

// QUICStreamHandler 处理QUIC连接的流
// 负责处理QUIC连接中的多个流，包括认证、加密通信和消息路由
type QUICStreamHandler struct {
	connection     quic.Connection               // QUIC连接实例
	streams        map[quic.StreamID]quic.Stream // 活跃流映射
	streamsMutex   sync.RWMutex                  // 保护streams映射的互斥锁
	converter      protocol.ProtocolConverter    // 协议转换器
	messageHandler MessageHandler                // 消息处理器
	cryptoManager  *security.CryptoManager       // 加密管理器
	sessionManager *SessionManager               // 会话管理器
	bufferSize     int                           // 读取缓冲区大小
	ctx            context.Context               // 处理器上下文，用于控制生命周期
	cancel         context.CancelFunc            // 用于取消上下文的函数
	sessionID      string                        // 与此处理器关联的会话ID
	authenticated  bool                          // 客户端是否已认证
	authMutex      sync.RWMutex                  // 保护认证状态的互斥锁
}

// NewQUICStreamHandler 创建新的QUIC流处理器
// 参数:
// - conn: QUIC连接
// - converter: 协议转换器，用于不同消息格式的转换
// - handler: 消息处理器，处理业务逻辑
// - cryptoManager: 加密管理器，处理加密/解密
// - sessionManager: 会话管理器，管理用户会话
// - bufferSize: 流读取缓冲区大小
func NewQUICStreamHandler(
	conn quic.Connection,
	converter protocol.ProtocolConverter,
	handler MessageHandler,
	cryptoManager *security.CryptoManager,
	sessionManager *SessionManager,
	bufferSize int,
) *QUICStreamHandler {
	ctx, cancel := context.WithCancel(context.Background())

	return &QUICStreamHandler{
		connection:     conn,
		streams:        make(map[quic.StreamID]quic.Stream),
		converter:      converter,
		messageHandler: handler,
		cryptoManager:  cryptoManager,
		sessionManager: sessionManager,
		bufferSize:     bufferSize,
		ctx:            ctx,
		cancel:         cancel,
		authenticated:  false,
	}
}

// Handle 开始处理QUIC连接中的流
// 等待新流创建并为每个流启动单独的goroutine
func (h *QUICStreamHandler) Handle() {
	// 生成会话ID并创建会话
	h.sessionID = utils.GenerateSessionID()
	session := NewSessionWithQUIC(h.sessionID, protocol.ConnectionTypeQUIC, h.connection)
	h.sessionManager.AddSession(session)

	// 记录连接信息
	log.Printf("Handling QUIC connection from %s with session ID %s",
		h.connection.RemoteAddr().String(), h.sessionID)

	// 处理流，直到上下文取消或连接关闭
	for {
		select {
		case <-h.ctx.Done():
			// 上下文已取消，退出循环
			return
		default:
			// 接受新的双向流
			stream, err := h.connection.AcceptStream(h.ctx)
			if err != nil {
				if h.ctx.Err() != nil {
					// 上下文已取消，这是预期的
					return
				}

				log.Printf("Error accepting QUIC stream: %v", err)
				// 如果是临时错误，继续尝试；否则退出
				if isTemporaryError(err) {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				return
			}

			// 将流添加到管理的流列表
			h.streamsMutex.Lock()
			h.streams[stream.StreamID()] = stream
			h.streamsMutex.Unlock()

			// 启动goroutine处理此流
			go h.handleStream(stream)
		}
	}
}

// handleStream 处理单个QUIC流
// 读取消息，进行认证和解密，然后转发给消息处理器
func (h *QUICStreamHandler) handleStream(stream quic.Stream) {
	defer func() {
		// 关闭流并从映射中移除
		stream.Close()
		h.streamsMutex.Lock()
		delete(h.streams, stream.StreamID())
		h.streamsMutex.Unlock()

		log.Printf("QUIC stream %d closed", stream.StreamID())
	}()

	// 创建缓冲区读取流数据
	buffer := make([]byte, h.bufferSize)

	// 首先处理认证（如果尚未认证）
	if !h.isAuthenticated() {
		if err := h.performAuthentication(stream, buffer); err != nil {
			log.Printf("Authentication failed: %v", err)
			return
		}

		// 标记为已认证
		h.setAuthenticated(true)
		log.Printf("Client authenticated for session %s", h.sessionID)
	}

	// 无限循环，读取并处理消息，直到流关闭或出错
	for {
		// 1. 读取消息长度（4字节整数）
		lenBuf := make([]byte, 4)
		_, err := io.ReadFull(stream, lenBuf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading message length: %v", err)
			}
			return
		}

		// 解析消息长度
		msgLen := binary.BigEndian.Uint32(lenBuf)
		if msgLen > uint32(h.bufferSize) {
			log.Printf("Message too large: %d bytes", msgLen)
			return
		}

		// 2. 读取消息内容
		msgBuf := make([]byte, msgLen)
		_, err = io.ReadFull(stream, msgBuf)
		if err != nil {
			log.Printf("Error reading message content: %v", err)
			return
		}

		// 3. 解密消息（如果已经建立了加密通道）
		var decryptedMsg []byte
		session, exists := h.sessionManager.GetSession(h.sessionID)
		if exists && session != nil {
			// 获取会话密钥 - 假设会话中存储了密钥
			// TODO: 实现加解密逻辑，目前只是简单示例
			// 实际中需要根据安全管理器的接口调整
			decryptedMsg = msgBuf
		} else {
			// 未加密的消息（例如，握手期间）
			decryptedMsg = msgBuf
		}

		// 4. 将二进制消息转换为协议消息
		msg, err := h.converter.ToInternal(protocol.ConnectionTypeQUIC, protocol.FormatTypeBinary, decryptedMsg)
		if err != nil {
			log.Printf("Failed to convert binary to message: %v", err)
			continue
		}

		// 设置消息的会话ID
		msg.SessionID = h.sessionID

		// 5. 更新会话的最后活动时间
		if session != nil {
			session.UpdateLastActiveTime()
		}

		// 6. 将消息传递给消息处理器
		response, err := h.messageHandler.HandleMessage(msg)
		if err != nil {
			log.Printf("Error handling message: %v", err)
			continue
		}

		// 7. 如果有响应，发送回客户端
		if response != nil {
			if err := h.sendResponse(stream, response, session); err != nil {
				log.Printf("Failed to send response: %v", err)
			}
		}
	}
}

// performAuthentication 执行客户端认证
// 实现安全握手和密钥协商过程
func (h *QUICStreamHandler) performAuthentication(stream quic.Stream, buffer []byte) error {
	// 读取认证消息
	n, err := stream.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read authentication message: %w", err)
	}

	// 解析认证消息
	authMsg, err := h.converter.ToInternal(protocol.ConnectionTypeQUIC, protocol.FormatTypeBinary, buffer[:n])
	if err != nil {
		return fmt.Errorf("failed to parse authentication message: %w", err)
	}

	// 使用authMsg进行认证
	h.sessionID = authMsg.SessionID
	if h.sessionID == "" {
		h.sessionID = utils.GenerateSessionID()
	}

	// 记录认证信息
	session := h.sessionManager.GetOrCreate(h.sessionID)
	if session == nil {
		return fmt.Errorf("failed to create session")
	}

	// 模拟认证过程 - 实际实现需要根据安全策略调整
	log.Printf("Client authenticated for session: %s", h.sessionID)

	// 创建认证响应
	authResponse := &protocol.Message{
		ServiceType: 1, // 认证服务类型
		MessageType: 2, // 认证响应消息类型
		SessionID:   h.sessionID,
		Payload:     []byte(`{"status":"authenticated"}`),
	}

	// 将响应发送回客户端
	responseBinary, err := h.converter.FromInternal(protocol.ConnectionTypeQUIC, protocol.FormatTypeBinary, authResponse)
	if err != nil {
		return fmt.Errorf("failed to convert auth response to binary: %w", err)
	}

	// 写入响应长度和内容
	lenBuf := make([]byte, 4)
	// 使用正确的方式设置长度（大端字节序）
	lenBuf[0] = byte(len(responseBinary) >> 24)
	lenBuf[1] = byte(len(responseBinary) >> 16)
	lenBuf[2] = byte(len(responseBinary) >> 8)
	lenBuf[3] = byte(len(responseBinary))

	if _, err := stream.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write auth response length: %w", err)
	}

	if _, err := stream.Write(responseBinary); err != nil {
		return fmt.Errorf("failed to write auth response: %w", err)
	}

	return nil
}

// sendResponse 将响应消息发送回客户端
// 将消息转换为二进制，加密（如果需要），并写入流
func (h *QUICStreamHandler) sendResponse(
	stream quic.Stream,
	response *protocol.Message,
	session *Session,
) error {
	// 将消息转换为二进制
	binary, err := h.converter.FromInternal(protocol.ConnectionTypeQUIC, protocol.FormatTypeBinary, response)
	if err != nil {
		return fmt.Errorf("failed to convert message to binary: %w", err)
	}

	// 消息现已准备好发送
	finalBinary := binary

	// 写入消息长度（4字节整数）
	lenBuf := make([]byte, 4)
	// 使用正确的方式设置长度（大端字节序）
	lenBuf[0] = byte(len(finalBinary) >> 24)
	lenBuf[1] = byte(len(finalBinary) >> 16)
	lenBuf[2] = byte(len(finalBinary) >> 8)
	lenBuf[3] = byte(len(finalBinary))

	if _, err := stream.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	// 写入消息内容
	if _, err := stream.Write(finalBinary); err != nil {
		return fmt.Errorf("failed to write message content: %w", err)
	}

	return nil
}

// Close 关闭流处理器，释放所有资源
func (h *QUICStreamHandler) Close() error {
	// 取消上下文，通知所有goroutine退出
	h.cancel()

	// 关闭所有活跃流
	h.streamsMutex.Lock()
	for id, stream := range h.streams {
		stream.Close()
		delete(h.streams, id)
	}
	h.streamsMutex.Unlock()

	// 如果有会话，移除它
	if h.sessionID != "" {
		h.sessionManager.RemoveSession(h.sessionID)
	}

	return nil
}

// isAuthenticated 检查客户端是否已认证
func (h *QUICStreamHandler) isAuthenticated() bool {
	h.authMutex.RLock()
	defer h.authMutex.RUnlock()
	return h.authenticated
}

// setAuthenticated 设置认证状态
func (h *QUICStreamHandler) setAuthenticated(authenticated bool) {
	h.authMutex.Lock()
	defer h.authMutex.Unlock()
	h.authenticated = authenticated
}

// isTemporaryError 检查错误是否为临时错误
func isTemporaryError(err error) bool {
	// 实现特定于QUIC的临时错误检测
	// 例如，超时或资源暂时不可用
	return false
}
