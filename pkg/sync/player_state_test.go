package sync

import (
	"context"
	"testing"
	"time"

	pb "mmo-server/proto_define"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlayerStateManager 测试玩家状态管理基本功能
func TestPlayerStateManager(t *testing.T) {
	manager := NewPlayerStateManager()

	// 测试注册玩家
	player1 := manager.RegisterPlayer("player1")
	assert.NotNil(t, player1, "注册的玩家不应为nil")
	assert.Equal(t, "player1", player1.ID, "玩家ID应该匹配")

	// 测试获取玩家
	player, err := manager.GetPlayer("player1")
	assert.NoError(t, err, "获取已注册玩家不应该返回错误")
	assert.Equal(t, player1, player, "获取的玩家应该是相同的实例")

	// 测试获取不存在的玩家
	_, err = manager.GetPlayer("nonexistent")
	assert.Error(t, err, "获取不存在的玩家应该返回错误")

	// 测试更新玩家位置
	newPos := &pb.Position{X: 10.0, Y: 5.0, Z: 15.0}
	err = manager.UpdatePlayerPosition("player1", newPos)
	assert.NoError(t, err, "更新玩家位置不应该返回错误")

	player, _ = manager.GetPlayer("player1")
	assert.Equal(t, newPos, player.Position, "玩家位置应该被更新")
	assert.Equal(t, uint32(UpdatePosition), player.UpdateMask&UpdatePosition, "位置更新标志应该被设置")

	// 测试更新玩家速度
	newVel := &pb.Vector3{X: 1.0, Y: 0.0, Z: 2.0}
	err = manager.UpdatePlayerVelocity("player1", newVel)
	assert.NoError(t, err, "更新玩家速度不应该返回错误")

	player, _ = manager.GetPlayer("player1")
	assert.Equal(t, newVel, player.Velocity, "玩家速度应该被更新")
	assert.Equal(t, uint32(UpdatePosition|UpdateVelocity),
		player.UpdateMask&(UpdatePosition|UpdateVelocity),
		"位置和速度更新标志应该被设置")

	// 测试更新玩家旋转角度
	err = manager.UpdatePlayerRotation("player1", 45.0)
	assert.NoError(t, err, "更新玩家旋转角度不应该返回错误")

	player, _ = manager.GetPlayer("player1")
	assert.Equal(t, float32(45.0), player.Rotation, "玩家旋转角度应该被更新")

	// 测试更新玩家属性
	err = manager.UpdatePlayerAttribute("player1", "hp", 100)
	assert.NoError(t, err, "更新玩家属性不应该返回错误")

	player, _ = manager.GetPlayer("player1")
	assert.Equal(t, 100, player.Attributes["hp"], "玩家属性应该被更新")

	// 测试注销玩家
	manager.UnregisterPlayer("player1")
	_, err = manager.GetPlayer("player1")
	assert.Error(t, err, "获取已注销的玩家应该返回错误")
}

// TestPlayerSubscription 测试玩家状态订阅机制
func TestPlayerSubscription(t *testing.T) {
	manager := NewPlayerStateManager()

	// 注册两个玩家
	manager.RegisterPlayer("player1")
	manager.RegisterPlayer("player2")

	// 测试订阅
	err := manager.SubscribeToPlayer("player2", "player1")
	assert.NoError(t, err, "订阅其他玩家不应该返回错误")

	// 验证订阅列表
	manager.mu.RLock()
	subscribers := manager.subscribers["player1"]
	manager.mu.RUnlock()

	assert.Contains(t, subscribers, "player2", "订阅列表应该包含订阅者")

	// 测试取消订阅
	err = manager.UnsubscribeFromPlayer("player2", "player1")
	assert.NoError(t, err, "取消订阅不应该返回错误")

	// 验证订阅列表已更新
	manager.mu.RLock()
	subscribers = manager.subscribers["player1"]
	manager.mu.RUnlock()

	assert.NotContains(t, subscribers, "player2", "订阅列表不应该包含已取消订阅的玩家")

	// 测试订阅不存在的玩家
	err = manager.SubscribeToPlayer("player1", "nonexistent")
	assert.Error(t, err, "订阅不存在的玩家应该返回错误")
}

// TestNotifyCallbacks 测试状态更新回调机制
func TestNotifyCallbacks(t *testing.T) {
	manager := NewPlayerStateManager()

	// 注册玩家
	manager.RegisterPlayer("player1")

	// 测试回调
	callbackCalled := false
	callbackPlayerID := ""

	manager.RegisterCallback("testCallback", func(playerID string, state *PlayerState) error {
		callbackCalled = true
		callbackPlayerID = playerID
		return nil
	})

	// 更新玩家触发回调
	err := manager.UpdatePlayerPosition("player1", &pb.Position{X: 10.0, Y: 0.0, Z: 0.0})
	assert.NoError(t, err, "更新玩家位置不应该返回错误")

	// 验证回调被调用
	assert.True(t, callbackCalled, "回调函数应该被调用")
	assert.Equal(t, "player1", callbackPlayerID, "回调函数应该接收正确的玩家ID")

	// 测试取消注册回调
	callbackCalled = false
	manager.UnregisterCallback("testCallback")

	// 再次更新玩家不应该触发回调
	err = manager.UpdatePlayerPosition("player1", &pb.Position{X: 20.0, Y: 0.0, Z: 0.0})
	assert.NoError(t, err)
	assert.False(t, callbackCalled, "取消注册后回调不应被调用")
}

// TestSyncPlayerStateService 测试玩家状态同步服务
func TestSyncPlayerStateService(t *testing.T) {
	manager := NewPlayerStateManager()
	service := NewSyncPlayerStateService(manager)

	// 注册一个玩家
	manager.RegisterPlayer("player1")

	// 创建同步请求
	req := &pb.PositionSyncRequest{
		Header: &pb.MessageHeader{
			MsgId:     int32(pb.MessageType_POSITION_SYNC_REQUEST),
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			SessionId: "test-session",
			Version:   1,
		},
		EntityId: "player1",
		Position: &pb.Position{
			X: 100.0,
			Y: 50.0,
			Z: 200.0,
		},
		Velocity: &pb.Vector3{
			X: 5.0,
			Y: 0.0,
			Z: 10.0,
		},
		Rotation:  90.0,
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}

	// 创建响应对象
	resp := &pb.PositionSyncResponse{}

	// 调用同步服务
	ctx := context.Background()
	err := service.SyncPosition(ctx, req, resp)
	require.NoError(t, err, "同步位置不应该返回错误")

	// 验证响应
	assert.True(t, resp.Success, "同步应该成功")
	assert.Equal(t, int32(0), resp.Header.ResultCode, "结果代码应该为0")

	// 验证玩家状态已更新
	player, _ := manager.GetPlayer("player1")
	assert.Equal(t, req.Position, player.Position, "玩家位置应该被更新")
	assert.Equal(t, req.Velocity, player.Velocity, "玩家速度应该被更新")
	assert.Equal(t, req.Rotation, player.Rotation, "玩家旋转角度应该被更新")
}

// TestBroadcastService 测试广播服务
func TestBroadcastService(t *testing.T) {
	manager := NewPlayerStateManager()
	service := NewBroadcastService(manager)

	// 注册几个玩家
	manager.RegisterPlayer("player1")
	manager.RegisterPlayer("player2")
	manager.RegisterPlayer("player3")

	// 创建广播请求
	req := &pb.ChatRequest{
		Header: &pb.MessageHeader{
			MsgId:     int32(pb.MessageType_CHAT_REQUEST),
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			SessionId: "test-session",
			Version:   1,
		},
		SenderId:   "player1",
		SenderName: "TestPlayer1",
		Content:    "Hello, World!",
		ChatType:   pb.ChatType_WORLD,
	}

	// 创建响应对象
	resp := &pb.ChatResponse{}

	// 调用广播服务
	ctx := context.Background()
	err := service.BroadcastMessage(ctx, req, resp)
	require.NoError(t, err, "广播消息不应该返回错误")

	// 验证响应
	assert.True(t, resp.Delivered, "消息应该被标记为已送达")
	assert.NotZero(t, resp.MessageId, "消息ID应该非零")
	assert.NotZero(t, resp.Timestamp, "时间戳应该非零")
	assert.Equal(t, int32(0), resp.Header.ResultCode, "结果代码应该为0")

	// 测试组播
	req.ChatType = pb.ChatType_TEAM
	req.Content = "Team message"

	resp = &pb.ChatResponse{}
	err = service.GroupcastMessage(ctx, req, resp)
	require.NoError(t, err, "组播消息不应该返回错误")

	// 验证响应
	assert.True(t, resp.Delivered, "消息应该被标记为已送达")
	assert.NotZero(t, resp.MessageId, "消息ID应该非零")
	assert.Contains(t, resp.Header.ResultMsg, "players in TEAM", "结果消息应该包含组类型")
}
