package network

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"

	pb "mmo-server/proto_define" // Import the generated protobuf code

	"google.golang.org/protobuf/proto"
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
	server.RegisterHandler(int32(pb.MessageType_LOGIN_REQUEST), processor.HandleLoginRequest)
	server.RegisterHandler(int32(pb.MessageType_HEARTBEAT_REQUEST), processor.HandleHeartbeatRequest)
	server.RegisterHandler(int32(pb.MessageType_POSITION_SYNC_REQUEST), processor.HandlePositionSyncRequest)
	server.RegisterHandler(int32(pb.MessageType_CHAT_REQUEST), processor.HandleChatRequest)
	server.RegisterHandler(int32(pb.MessageType_COMBAT_COMMAND_REQUEST), processor.HandleCombatCommandRequest)

	return processor
}

// createResponseHeader 创建响应消息头
func createResponseHeader(reqHeader *pb.MessageHeader, msgID int32) *pb.MessageHeader {
	return &pb.MessageHeader{
		MsgId:      msgID,
		Timestamp:  time.Now().UnixNano() / 1e6, // 毫秒
		SessionId:  reqHeader.SessionId,
		Version:    reqHeader.Version,
		ResultCode: 0, // 成功
		ResultMsg:  "success",
	}
}

// intToBytes 将int32转换为4字节的字节数组
func intToBytes(val int32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, uint32(val))
	return bytes
}

// HandleLoginRequest 处理登录请求
func (p *MessageProcessor) HandleLoginRequest(client *Client, message []byte) error {
	var request pb.LoginRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal login request error: %w", err)
	}

	log.Printf("Received login request from user: %s", request.Username)

	// 验证登录并生成响应
	// 在实际应用中，应该验证用户名密码，生成令牌等
	response := &pb.LoginResponse{
		Header: createResponseHeader(request.Header, int32(pb.MessageType_LOGIN_RESPONSE)),
		UserId: "user_" + request.Username,
		Token:  "mock_token_" + request.Username,
		UserInfo: &pb.UserInfo{
			UserId:        "user_" + request.Username,
			Username:      request.Username,
			Nickname:      "Player_" + request.Username,
			Level:         1,
			Exp:           0,
			VipLevel:      0,
			LastLoginTime: time.Now().Unix(),
			CreatedTime:   time.Now().Unix(),
		},
		ServerInfo: &pb.ServerInfo{
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
	msgIDBytes := intToBytes(int32(pb.MessageType_LOGIN_RESPONSE))

	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}

// HandleHeartbeatRequest 处理心跳请求
func (p *MessageProcessor) HandleHeartbeatRequest(client *Client, message []byte) error {
	var request pb.HeartbeatRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal heartbeat request error: %w", err)
	}

	response := &pb.HeartbeatResponse{
		Header:     createResponseHeader(request.Header, int32(pb.MessageType_HEARTBEAT_RESPONSE)),
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
	msgIDBytes := intToBytes(int32(pb.MessageType_HEARTBEAT_RESPONSE))

	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}

// HandlePositionSyncRequest 处理位置同步请求
func (p *MessageProcessor) HandlePositionSyncRequest(client *Client, message []byte) error {
	var request pb.PositionSyncRequest
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
	response := &pb.PositionSyncResponse{
		Header:  createResponseHeader(request.Header, int32(pb.MessageType_POSITION_SYNC_RESPONSE)),
		Success: true,
		// 模拟附近的实体
		NearbyEntities: []*pb.EntityPosition{
			{
				EntityId:   "npc_1",
				EntityType: "npc",
				Position: &pb.Position{
					X: request.Position.X + 10,
					Y: request.Position.Y,
					Z: request.Position.Z + 10,
				},
				Rotation: 0,
				Velocity: &pb.Vector3{X: 0, Y: 0, Z: 0},
			},
		},
	}

	// 序列化并发送响应
	responseData, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal position sync response error: %w", err)
	}

	// 添加消息ID
	msgIDBytes := intToBytes(int32(pb.MessageType_POSITION_SYNC_RESPONSE))

	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}

// HandleChatRequest 处理聊天请求
func (p *MessageProcessor) HandleChatRequest(client *Client, message []byte) error {
	var request pb.ChatRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal chat request error: %w", err)
	}

	log.Printf("Chat message from %s: %s", request.SenderName, request.Content)

	// 创建响应
	response := &pb.ChatResponse{
		Header:    createResponseHeader(request.Header, int32(pb.MessageType_CHAT_RESPONSE)),
		Delivered: true,
		MessageId: time.Now().UnixNano(),
		Timestamp: time.Now().Unix(),
	}

	// 根据聊天类型处理消息
	switch request.ChatType {
	case pb.ChatType_WORLD:
		// 广播到所有客户端
		// 序列化并创建广播消息
		broadcastData, err := proto.Marshal(&request)
		if err != nil {
			return fmt.Errorf("marshal chat broadcast error: %w", err)
		}

		msgIDBytes := intToBytes(int32(pb.MessageType_CHAT_REQUEST))

		fullBroadcast := append(msgIDBytes, broadcastData...)
		p.server.Broadcast(fullBroadcast)

	case pb.ChatType_PRIVATE:
		// 在实际应用中，应该发送给指定用户
		log.Printf("Private message to %s", request.TargetId)
	}

	// 序列化并发送响应
	responseData, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal chat response error: %w", err)
	}

	// 添加消息ID
	msgIDBytes := intToBytes(int32(pb.MessageType_CHAT_RESPONSE))

	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}

// HandleCombatCommandRequest 处理战斗指令请求
func (p *MessageProcessor) HandleCombatCommandRequest(client *Client, message []byte) error {
	var request pb.CombatCommandRequest
	if err := proto.Unmarshal(message[4:], &request); err != nil {
		return fmt.Errorf("unmarshal combat command request error: %w", err)
	}

	log.Printf("Combat command from %s to %s, skill: %d",
		request.AttackerId,
		request.TargetId,
		request.SkillId,
	)

	// 创建响应，模拟战斗结果
	response := &pb.CombatCommandResponse{
		Header:  createResponseHeader(request.Header, int32(pb.MessageType_COMBAT_COMMAND_RESPONSE)),
		Success: true,
		Results: []*pb.CombatResult{
			{
				TargetId:    request.TargetId,
				Hit:         true,
				Critical:    false,
				Damage:      100,
				RemainingHp: 900,
				BuffEffects: []*pb.BuffEffect{
					{
						BuffId:   1,
						BuffName: "Bleed",
						Duration: 5,
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
	msgIDBytes := intToBytes(int32(pb.MessageType_COMBAT_COMMAND_RESPONSE))

	fullResponse := append(msgIDBytes, responseData...)
	return client.SendMessage(fullResponse)
}
