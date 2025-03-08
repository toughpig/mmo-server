package rpc

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "mmo-server/proto_define"
)

// PlayerService is an example service that provides player-related functionality
type PlayerService struct {
	playerPositions map[string]*pb.Vector3
}

// GetPlayerInfo retrieves information about a player
func (s *PlayerService) GetPlayerInfo(ctx context.Context, req *pb.LoginRequest, resp *pb.LoginResponse) error {
	log.Printf("GetPlayerInfo called for user: %s", req.Username)

	// Create a dummy response
	resp.UserId = "player123"
	resp.Token = "auth-token-xyz"
	resp.UserInfo = &pb.UserInfo{
		UserId:        "player123",
		Username:      req.Username,
		Nickname:      fmt.Sprintf("Player_%s", req.Username),
		Level:         10,
		Exp:           5000,
		VipLevel:      2,
		LastLoginTime: 1615471200000, // Unix timestamp in milliseconds
		CreatedTime:   1610000000000, // Unix timestamp in milliseconds
	}
	resp.ServerInfo = &pb.ServerInfo{
		ServerId:       "sv001",
		ServerName:     "Game Server 1",
		ServerStatus:   1, // 1 = online
		ServerAddress:  "game1.example.com",
		ServerPort:     8080,
		OnlineCount:    1250,
		MaxOnlineCount: 2000,
	}
	resp.Header = &pb.MessageHeader{
		MsgId:      int32(pb.MessageType_LOGIN_RESPONSE),
		Timestamp:  1615471200000, // Unix timestamp in milliseconds
		SessionId:  "session-123",
		Version:    1,
		ResultCode: 0, // Success
		ResultMsg:  "Success",
	}

	return nil
}

// SyncPlayerPosition synchronizes a player's position with other players
func (s *PlayerService) SyncPlayerPosition(ctx context.Context, req *pb.PositionSyncRequest, resp *pb.PositionSyncResponse) error {
	log.Printf("SyncPlayerPosition called for entity: %s", req.EntityId)

	// In a real implementation, we would:
	// 1. Update the player's position in our world state
	// 2. Query nearby entities from the AOI system
	// 3. Return information about nearby entities

	// For testing purposes, create a dummy response with some nearby entities
	resp.Success = true
	resp.NearbyEntities = []*pb.EntityPosition{
		{
			EntityId:   "npc-1",
			EntityType: "NPC",
			Position: &pb.Position{
				X: 100.5,
				Y: 0.0,
				Z: 200.3,
			},
			Rotation: 45.0,
		},
		{
			EntityId:   "player-456",
			EntityType: "PLAYER",
			Position: &pb.Position{
				X: 120.7,
				Y: 0.0,
				Z: 180.2,
			},
			Rotation: 90.0,
		},
	}
	resp.Header = &pb.MessageHeader{
		MsgId:      int32(pb.MessageType_POSITION_SYNC_RESPONSE),
		Timestamp:  time.Now().UnixNano() / int64(time.Millisecond),
		SessionId:  "session-123",
		Version:    1,
		ResultCode: 0, // Success
		ResultMsg:  "Success",
	}

	return nil
}

// UpdatePosition 处理玩家位置更新请求
func (s *PlayerService) UpdatePosition(ctx context.Context, req *pb.PlayerPositionRequest, resp *pb.PlayerPositionResponse) error {
	// 初始化map（如果需要）
	if s.playerPositions == nil {
		s.playerPositions = make(map[string]*pb.Vector3)
	}

	// 记录玩家位置
	s.playerPositions[req.PlayerId] = req.Position

	log.Printf("Updated position for player %s: (%f, %f, %f)",
		req.PlayerId, req.Position.X, req.Position.Y, req.Position.Z)

	// 设置响应
	resp.Success = true
	resp.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)

	// 如果需要，可以校正位置（这里只是示例）
	resp.CorrectedPosition = &pb.Vector3{
		X: req.Position.X,
		Y: req.Position.Y,
		Z: req.Position.Z,
	}

	// 设置响应中的附近实体（简化示例）
	// 在实际实现中，这应该通过AOI系统获取
	resp.NearbyEntities = []*pb.EntityPosition{
		{
			EntityId:   "npc-1",
			EntityType: "NPC",
			Position: &pb.Position{
				X: req.Position.X + 10.0,
				Y: req.Position.Y,
				Z: req.Position.Z + 10.0,
			},
			Rotation: 45.0,
		},
	}

	return nil
}

// SlowOperation 是一个用于测试超时的慢速方法
func (s *PlayerService) SlowOperation(ctx context.Context, req *pb.PlayerPositionRequest, resp *pb.PlayerPositionResponse) error {
	// 模拟耗时操作
	select {
	case <-time.After(2 * time.Second):
		resp.Success = true
		resp.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
		return nil
	case <-ctx.Done():
		// 上下文取消（超时或取消）
		return ctx.Err()
	}
}

// GetPlayerPosition 获取玩家的当前位置
func (s *PlayerService) GetPlayerPosition(ctx context.Context, req *pb.PlayerPositionRequest, resp *pb.PlayerPositionResponse) error {
	// 初始化map（如果需要）
	if s.playerPositions == nil {
		s.playerPositions = make(map[string]*pb.Vector3)
	}

	// 获取玩家位置
	pos, exists := s.playerPositions[req.PlayerId]
	if !exists {
		resp.Success = false
		resp.ErrorMessage = "Player position not found"
		return nil
	}

	// 设置响应
	resp.Success = true
	resp.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
	resp.CorrectedPosition = pos

	return nil
}

// StartExample starts a simple example that demonstrates both server and client
func StartExample() {
	// Create a gRPC endpoint
	endpoint := "localhost:50051"

	// Create and start a server
	server := NewRPCServer(endpoint, DefaultTransport)

	// Register the example service
	service := &PlayerService{}
	err := server.Register(service)
	if err != nil {
		log.Fatalf("Failed to register service: %v", err)
	}

	// Prepare the endpoint
	err = PrepareEndpoint(endpoint, DefaultTransport)
	if err != nil {
		log.Fatalf("Failed to prepare endpoint: %v", err)
	}

	// Start the server
	err = server.Start()
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Create a client
	client, err := NewRPCClient(endpoint, DefaultTransport)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create a direct PlayerService client
	playerClient, err := GetPlayerServiceClient(endpoint)
	if err != nil {
		log.Fatalf("Failed to create player service client: %v", err)
	}

	// 注意：playerClient 由连接管理器管理，不需要手动关闭
	// 在程序结束时，连接管理器会自动清理所有连接

	// Call GetPlayerInfo
	loginReq := &pb.LoginRequest{
		Username:      "testuser",
		Password:      "password123",
		DeviceId:      "device123",
		ClientVersion: "1.0.0",
		Header: &pb.MessageHeader{
			MsgId:      int32(pb.MessageType_LOGIN_REQUEST),
			Timestamp:  1615471200000,
			SessionId:  "session-123",
			Version:    1,
			ResultCode: 0,
			ResultMsg:  "",
		},
	}
	loginResp := &pb.LoginResponse{}

	ctx := context.Background()
	err = client.Call(ctx, "PlayerService.GetPlayerInfo", loginReq, loginResp)
	if err != nil {
		log.Printf("GetPlayerInfo error: %v", err)
	} else {
		log.Printf("GetPlayerInfo response: user_id=%s, token=%s, nickname=%s",
			loginResp.UserId, loginResp.Token, loginResp.UserInfo.Nickname)
	}

	// Example of using the direct gRPC client
	posReq := &pb.PlayerPositionRequest{
		PlayerId: "player123",
		Position: &pb.Vector3{
			X: 100.0,
			Y: 0.0,
			Z: 50.0,
		},
		Rotation: &pb.Vector3{
			X: 0.0,
			Y: 45.0,
			Z: 0.0,
		},
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}

	posResp, err := playerClient.UpdatePosition(ctx, posReq)
	if err != nil {
		log.Printf("UpdatePosition error: %v", err)
	} else {
		log.Printf("UpdatePosition response: success=%v, timestamp=%d",
			posResp.Success, posResp.Timestamp)
	}

	// 在测试结束时清理连接
	DefaultConnManager.CloseAll()

	log.Println("Example completed.")
}
