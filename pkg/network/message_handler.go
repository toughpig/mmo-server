package network

import (
	"fmt"
	"log"
	"time"

	"google.golang.org/protobuf/proto"
	"github.com/yourusername/mmo-server/proto"
)

// MessageProcessor 处理所有类型的消息
type MessageProcessor struct {
	server *Server
}

// NewMessageProcessor 创建新的消息处理器
func NewMessageProcessor(server *Server) *MessageProcessor {
	processor := &MessageProcessor{
		server: server,
	}
	
	// 注册消息处理函数
	server.RegisterHandler(int32(proto.MessageType_LOGIN_REQUEST), processor.HandleLoginRequest)
	server.RegisterHandler(int32(proto.MessageType_HEARTBEAT_REQUEST), processor.HandleHeartbeatRequest)
	server.RegisterHandler(int32(proto.MessageType_POSITION_SYNC_REQUEST), processor.HandlePositionSyncRequest)
	server.RegisterHandler(int32(proto.MessageType_CHAT_REQUEST), processor.HandleChatRequest)
	server.RegisterHandler(int32(proto.MessageType_COMBAT_COMMAND_REQUEST), processor.HandleCombatCommandRequest)
	
	return processor
}

// createResponseHeader 创建响应消息头
func createResponseHeader(reqHeader *proto.MessageHeader, msgID int32) *proto.MessageHeader {
	return &proto.MessageHeader{
		MsgId:      msgID,
		Timestamp:  time.Now().UnixNano() / 1e6, // 毫秒
		SessionId:  reqHeader.SessionId,
		Version:    reqHeader.Version,
		ResultCode: 0, // 成功
		ResultMsg:  "success",
	}
}

// HandleLoginRequest 处理登录请求
func (p *MessageProcessor) HandleLoginRequest(client *Client, message []byte) error {
	var request proto.LoginRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal login request error: %w", err)
	}
	
	log.Printf("Received login request from user: %s", request.Username)
	
	// 验证登录并生成响应
	// 在实际应用中，应该验证用户名密码，生成令牌等
	response := &proto.LoginResponse{
		Header: createResponseHeader(request.Header, int32(proto.MessageType_LOGIN_RESPONSE)),
		UserId: "user_" + request.Username,
		Token:  "mock_token_" + request.Username,
		UserInfo: &proto.UserInfo{
			UserId:        "user_" + request.Username,
			Username:      request.Username,
			Nickname:      "Player_" + request.Username,
			Level:         1,
			Exp:           0,
			VipLevel:      0,
			LastLoginTime: time.Now().Unix(),
			CreatedTime:   time.Now().Unix(),
		},
		ServerInfo: &proto.ServerInfo{
			ServerId:       "server_1",
			ServerName:     "Game Server 1",
			ServerStatus:   1,
			ServerAddress:  "127.0.0.1",
			ServerPort:     8080,
			OnlineCount:    100,
			MaxOnlineCount: 1000,
		},
	}
	
	// 序列化并发送响应
	responseData, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal login response error: %w", err)
	}
	
	// 添加消息ID（前4个字节）
	msgIDBytes := []byte{
		byte(proto.MessageType_LOGIN_RESPONSE >> 24),
		byte(proto.MessageType_LOGIN_RESPONSE >> 16),
		byte(proto.MessageType_LOGIN_RESPONSE >> 8),
		byte(proto.MessageType_LOGIN_RESPONSE),
	}
	
	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}

// HandleHeartbeatRequest 处理心跳请求
func (p *MessageProcessor) HandleHeartbeatRequest(client *Client, message []byte) error {
	var request proto.HeartbeatRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal heartbeat request error: %w", err)
	}
	
	response := &proto.HeartbeatResponse{
		Header:     createResponseHeader(request.Header, int32(proto.MessageType_HEARTBEAT_RESPONSE)),
		ServerTime: time.Now().UnixNano() / 1e6, // 毫秒
	}
	
	// 更新客户端最后心跳时间
	client.lastPing = time.Now()
	
	// 序列化并发送响应
	responseData, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal heartbeat response error: %w", err)
	}
	
	// 添加消息ID
	msgIDBytes := []byte{
		byte(proto.MessageType_HEARTBEAT_RESPONSE >> 24),
		byte(proto.MessageType_HEARTBEAT_RESPONSE >> 16),
		byte(proto.MessageType_HEARTBEAT_RESPONSE >> 8),
		byte(proto.MessageType_HEARTBEAT_RESPONSE),
	}
	
	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}

// HandlePositionSyncRequest 处理位置同步请求
func (p *MessageProcessor) HandlePositionSyncRequest(client *Client, message []byte) error {
	var request proto.PositionSyncRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal position sync request error: %w", err)
	}
	
	// 处理位置更新，在实际应用中应该更新实体位置并执行碰撞检测等
	log.Printf("Entity %s updated position to (%f, %f, %f)",
		request.EntityId,
		request.Position.X,
		request.Position.Y,
		request.Position.Z,
	)
	
	// 创建响应，包含附近实体位置
	response := &proto.PositionSyncResponse{
		Header:  createResponseHeader(request.Header, int32(proto.MessageType_POSITION_SYNC_RESPONSE)),
		Success: true,
		// 模拟附近的实体
		NearbyEntities: []*proto.EntityPosition{
			{
				EntityId:   "npc_1",
				EntityType: "npc",
				Position: &proto.Position{
					X: request.Position.X + 10,
					Y: request.Position.Y,
					Z: request.Position.Z + 10,
				},
				Rotation: 0,
				Velocity: &proto.Vector3{X: 0, Y: 0, Z: 0},
			},
		},
	}
	
	// 序列化并发送响应
	responseData, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal position sync response error: %w", err)
	}
	
	// 添加消息ID
	msgIDBytes := []byte{
		byte(proto.MessageType_POSITION_SYNC_RESPONSE >> 24),
		byte(proto.MessageType_POSITION_SYNC_RESPONSE >> 16),
		byte(proto.MessageType_POSITION_SYNC_RESPONSE >> 8),
		byte(proto.MessageType_POSITION_SYNC_RESPONSE),
	}
	
	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}

// HandleChatRequest 处理聊天请求
func (p *MessageProcessor) HandleChatRequest(client *Client, message []byte) error {
	var request proto.ChatRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal chat request error: %w", err)
	}
	
	log.Printf("Chat message from %s: %s", request.SenderName, request.Content)
	
	// 创建响应
	response := &proto.ChatResponse{
		Header:    createResponseHeader(request.Header, int32(proto.MessageType_CHAT_RESPONSE)),
		Delivered: true,
		MessageId: time.Now().UnixNano(),
		Timestamp: time.Now().Unix(),
	}
	
	// 根据聊天类型处理消息
	switch request.ChatType {
	case proto.ChatType_WORLD:
		// 广播到所有客户端
		// 序列化并创建广播消息
		broadcastData, err := proto.Marshal(&request)
		if err != nil {
			return fmt.Errorf("marshal chat broadcast error: %w", err)
		}
		
		msgIDBytes := []byte{
			byte(proto.MessageType_CHAT_REQUEST >> 24),
			byte(proto.MessageType_CHAT_REQUEST >> 16),
			byte(proto.MessageType_CHAT_REQUEST >> 8),
			byte(proto.MessageType_CHAT_REQUEST),
		}
		
		fullBroadcast := append(msgIDBytes, broadcastData...)
		p.server.Broadcast(fullBroadcast)
		
	case proto.ChatType_PRIVATE:
		// 在实际应用中，应该发送给指定用户
		log.Printf("Private message to %s", request.TargetId)
	}
	
	// 序列化并发送响应
	responseData, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal chat response error: %w", err)
	}
	
	// 添加消息ID
	msgIDBytes := []byte{
		byte(proto.MessageType_CHAT_RESPONSE >> 24),
		byte(proto.MessageType_CHAT_RESPONSE >> 16),
		byte(proto.MessageType_CHAT_RESPONSE >> 8),
		byte(proto.MessageType_CHAT_RESPONSE),
	}
	
	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}

// HandleCombatCommandRequest 处理战斗指令请求
func (p *MessageProcessor) HandleCombatCommandRequest(client *Client, message []byte) error {
	var request proto.CombatCommandRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal combat command request error: %w", err)
	}
	
	log.Printf("Combat command from %s to %s using skill %d",
		request.AttackerId,
		request.TargetId,
		request.SkillId,
	)
	
	// 创建响应，模拟战斗结果
	response := &proto.CombatCommandResponse{
		Header:  createResponseHeader(request.Header, int32(proto.MessageType_COMBAT_COMMAND_RESPONSE)),
		Success: true,
		Results: []*proto.CombatResult{
			{
				TargetId:    request.TargetId,
				Hit:         true,
				Critical:    false,
				Damage:      100,
				RemainingHp: 900,
				BuffEffects: []*proto.BuffEffect{
					{
						BuffId:   1,
						BuffName: "Stunned",
						Duration: 2,
						Stacks:   1,
					},
				},
			},
		},
	}
	
	// 序列化并发送响应
	responseData, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal combat command response error: %w", err)
	}
	
	// 添加消息ID
	msgIDBytes := []byte{
		byte(proto.MessageType_COMBAT_COMMAND_RESPONSE >> 24),
		byte(proto.MessageType_COMBAT_COMMAND_RESPONSE >> 16),
		byte(proto.MessageType_COMBAT_COMMAND_RESPONSE >> 8),
		byte(proto.MessageType_COMBAT_COMMAND_RESPONSE),
	}
	
	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
} 