package sync

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"mmo-server/pkg/aoi"
	pb "mmo-server/proto_define"
)

// PlayerState 表示一个玩家的状态
type PlayerState struct {
	ID             string                 // 玩家唯一ID
	Position       *pb.Position           // 位置
	Velocity       *pb.Vector3            // 速度
	Rotation       float32                // 旋转角度
	LastSyncTime   int64                  // 最后同步时间
	LastUpdateTime int64                  // 最后更新时间
	Attributes     map[string]interface{} // 玩家属性（例如HP, MP等）
	UpdateMask     uint32                 // 标记哪些字段已更新
	mu             sync.RWMutex           // 保护并发访问
}

// GetID 实现AOI实体接口
func (p *PlayerState) GetID() string {
	return p.ID
}

// GetPosition 实现AOI实体接口
func (p *PlayerState) GetPosition() (float32, float32, float32) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Position.X, p.Position.Y, p.Position.Z
}

// GetTypeID 实现AOI实体接口，玩家类型为0
func (p *PlayerState) GetTypeID() int {
	return 0
}

// PlayerStateManager 管理所有玩家的状态
type PlayerStateManager struct {
	players     map[string]*PlayerState        // 所有玩家的状态
	subscribers map[string][]string            // 订阅者 (玩家ID -> 订阅者ID数组)
	callbacks   map[string]StateUpdateCallback // 状态更新回调
	aoiManager  *aoi.AOIManager                // 兴趣区域管理器
	mu          sync.RWMutex
}

// StateUpdateCallback 是一个回调函数，当玩家状态更新时被调用
type StateUpdateCallback func(playerID string, state *PlayerState) error

// NewPlayerStateManager 创建一个新的玩家状态管理器
func NewPlayerStateManager() *PlayerStateManager {
	// 创建一个1000x1000的游戏世界，20x20的网格
	aoiManager := aoi.NewAOIManager(0, 1000, 0, 1000, 20)

	return &PlayerStateManager{
		players:     make(map[string]*PlayerState),
		subscribers: make(map[string][]string),
		callbacks:   make(map[string]StateUpdateCallback),
		aoiManager:  aoiManager,
	}
}

// RegisterPlayer 注册一个新玩家
func (m *PlayerStateManager) RegisterPlayer(playerID string) *PlayerState {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查玩家是否已存在
	if player, exists := m.players[playerID]; exists {
		return player
	}

	// 创建新玩家状态
	now := time.Now().UnixNano() / int64(time.Millisecond)
	player := &PlayerState{
		ID: playerID,
		Position: &pb.Position{
			X: 0,
			Y: 0,
			Z: 0,
		},
		Velocity: &pb.Vector3{
			X: 0,
			Y: 0,
			Z: 0,
		},
		Rotation:       0,
		LastSyncTime:   now,
		LastUpdateTime: now,
		Attributes:     make(map[string]interface{}),
	}

	// 存储玩家状态
	m.players[playerID] = player
	m.subscribers[playerID] = []string{}

	// 将玩家添加到AOI系统
	err := m.aoiManager.AddEntity(player)
	if err != nil {
		log.Printf("将玩家添加到AOI系统失败：%v", err)
	}

	log.Printf("Player registered: %s", playerID)
	return player
}

// UnregisterPlayer 注销一个玩家
func (m *PlayerStateManager) UnregisterPlayer(playerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取玩家
	player, exists := m.players[playerID]
	if !exists {
		return
	}

	// 从AOI系统中移除
	err := m.aoiManager.RemoveEntity(player)
	if err != nil {
		log.Printf("从AOI系统中移除玩家失败：%v", err)
	}

	// 删除玩家状态
	delete(m.players, playerID)

	// 删除所有订阅
	delete(m.subscribers, playerID)

	// 从所有订阅列表中移除这个玩家
	for otherID, subs := range m.subscribers {
		var newSubs []string
		for _, subID := range subs {
			if subID != playerID {
				newSubs = append(newSubs, subID)
			}
		}
		m.subscribers[otherID] = newSubs
	}

	log.Printf("Player unregistered: %s", playerID)
}

// GetPlayer 获取玩家状态
func (m *PlayerStateManager) GetPlayer(playerID string) (*PlayerState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	player, exists := m.players[playerID]
	if !exists {
		return nil, fmt.Errorf("player not found: %s", playerID)
	}

	return player, nil
}

// 更新掩码常量
const (
	UpdatePosition = 1 << iota
	UpdateVelocity
	UpdateRotation
	UpdateAttributes
)

// UpdatePlayerPosition 更新玩家位置
func (m *PlayerStateManager) UpdatePlayerPosition(playerID string, position *pb.Position) error {
	player, err := m.GetPlayer(playerID)
	if err != nil {
		return err
	}

	// 保存旧位置用于AOI更新
	oldX, _, oldZ := player.GetPosition()

	player.mu.Lock()
	player.Position = position
	player.LastUpdateTime = time.Now().UnixNano() / int64(time.Millisecond)
	player.UpdateMask |= UpdatePosition
	player.mu.Unlock()

	// 更新AOI中的位置
	err = m.aoiManager.UpdateEntity(player, oldX, oldZ)
	if err != nil {
		log.Printf("更新AOI中的玩家位置失败：%v", err)
	}

	return m.notifySubscribers(playerID)
}

// UpdatePlayerVelocity 更新玩家速度
func (m *PlayerStateManager) UpdatePlayerVelocity(playerID string, velocity *pb.Vector3) error {
	player, err := m.GetPlayer(playerID)
	if err != nil {
		return err
	}

	player.mu.Lock()
	player.Velocity = velocity
	player.LastUpdateTime = time.Now().UnixNano() / int64(time.Millisecond)
	player.UpdateMask |= UpdateVelocity
	player.mu.Unlock()

	return m.notifySubscribers(playerID)
}

// UpdatePlayerRotation 更新玩家旋转角度
func (m *PlayerStateManager) UpdatePlayerRotation(playerID string, rotation float32) error {
	player, err := m.GetPlayer(playerID)
	if err != nil {
		return err
	}

	player.mu.Lock()
	player.Rotation = rotation
	player.LastUpdateTime = time.Now().UnixNano() / int64(time.Millisecond)
	player.UpdateMask |= UpdateRotation
	player.mu.Unlock()

	return m.notifySubscribers(playerID)
}

// UpdatePlayerAttribute 更新玩家属性
func (m *PlayerStateManager) UpdatePlayerAttribute(playerID string, key string, value interface{}) error {
	player, err := m.GetPlayer(playerID)
	if err != nil {
		return err
	}

	player.mu.Lock()
	player.Attributes[key] = value
	player.LastUpdateTime = time.Now().UnixNano() / int64(time.Millisecond)
	player.UpdateMask |= UpdateAttributes
	player.mu.Unlock()

	return m.notifySubscribers(playerID)
}

// SubscribeToPlayer 订阅玩家状态更新
func (m *PlayerStateManager) SubscribeToPlayer(subscriberID, targetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查目标玩家是否存在
	if _, exists := m.players[targetID]; !exists {
		return fmt.Errorf("target player not found: %s", targetID)
	}

	// 添加到订阅列表
	subscribers := m.subscribers[targetID]
	for _, id := range subscribers {
		if id == subscriberID {
			// 已经订阅了
			return nil
		}
	}

	m.subscribers[targetID] = append(subscribers, subscriberID)
	log.Printf("Player %s subscribed to %s", subscriberID, targetID)
	return nil
}

// UnsubscribeFromPlayer 取消订阅玩家状态更新
func (m *PlayerStateManager) UnsubscribeFromPlayer(subscriberID, targetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscribers, exists := m.subscribers[targetID]
	if !exists {
		return fmt.Errorf("target player not found: %s", targetID)
	}

	// 从订阅列表中移除
	var newSubscribers []string
	for _, id := range subscribers {
		if id != subscriberID {
			newSubscribers = append(newSubscribers, id)
		}
	}

	m.subscribers[targetID] = newSubscribers
	log.Printf("Player %s unsubscribed from %s", subscriberID, targetID)
	return nil
}

// RegisterCallback 注册状态更新回调
func (m *PlayerStateManager) RegisterCallback(name string, callback StateUpdateCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks[name] = callback
}

// UnregisterCallback 注销状态更新回调
func (m *PlayerStateManager) UnregisterCallback(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.callbacks, name)
}

// notifySubscribers 通知所有订阅者玩家状态已更新
func (m *PlayerStateManager) notifySubscribers(playerID string) error {
	m.mu.RLock()
	subscribers := m.subscribers[playerID]
	player := m.players[playerID]
	callbacks := make(map[string]StateUpdateCallback)
	for name, callback := range m.callbacks {
		callbacks[name] = callback
	}
	m.mu.RUnlock()

	// 通知所有订阅者
	for _, subscriberID := range subscribers {
		// 这里可以添加发送状态更新的逻辑
		log.Printf("Notify player %s about state change of %s", subscriberID, playerID)
	}

	// 调用所有回调
	for name, callback := range callbacks {
		if err := callback(playerID, player); err != nil {
			log.Printf("Error in callback %s: %v", name, err)
		}
	}

	return nil
}

// GetNearbyPlayers 获取玩家附近的玩家数量
func (m *PlayerStateManager) GetNearbyPlayers(playerID string, range_ int) (int, error) {
	player, err := m.GetPlayer(playerID)
	if err != nil {
		return 0, err
	}

	x, _, z := player.GetPosition()
	return m.aoiManager.GetNearbyPlayers(x, z, range_)
}

// SyncPlayerStateService 同步玩家状态（基于RPC服务）
type SyncPlayerStateService struct {
	manager *PlayerStateManager
}

// NewSyncPlayerStateService 创建一个新的玩家状态同步服务
func NewSyncPlayerStateService(manager *PlayerStateManager) *SyncPlayerStateService {
	return &SyncPlayerStateService{
		manager: manager,
	}
}

// SyncPosition 处理位置同步请求
func (s *SyncPlayerStateService) SyncPosition(ctx context.Context, req *pb.PositionSyncRequest, resp *pb.PositionSyncResponse) error {
	log.Printf("SyncPosition called for entity: %s at position: (%f,%f,%f)",
		req.EntityId,
		req.Position.X,
		req.Position.Y,
		req.Position.Z)

	// 更新玩家位置
	err := s.manager.UpdatePlayerPosition(req.EntityId, req.Position)
	if err != nil {
		resp.Success = false
		resp.Header = &pb.MessageHeader{
			MsgId:      int32(pb.MessageType_POSITION_SYNC_RESPONSE),
			Timestamp:  time.Now().UnixNano() / int64(time.Millisecond),
			SessionId:  req.Header.SessionId,
			Version:    req.Header.Version,
			ResultCode: 1, // Error
			ResultMsg:  err.Error(),
		}
		return err
	}

	// 如果有速度信息，也更新
	if req.Velocity != nil {
		s.manager.UpdatePlayerVelocity(req.EntityId, req.Velocity)
	}

	// 更新旋转角度
	s.manager.UpdatePlayerRotation(req.EntityId, req.Rotation)

	// 获取附近实体
	nearbyEntities, err := s.getNearbyEntities(req.EntityId)
	if err != nil {
		nearbyEntities = []*pb.EntityPosition{} // 空数组而不是nil
	}

	// 构建响应
	resp.Success = true
	resp.NearbyEntities = nearbyEntities
	resp.Header = &pb.MessageHeader{
		MsgId:      int32(pb.MessageType_POSITION_SYNC_RESPONSE),
		Timestamp:  req.Timestamp + 50, // 添加50ms作为服务器处理时间
		SessionId:  req.Header.SessionId,
		Version:    req.Header.Version,
		ResultCode: 0, // Success
		ResultMsg:  "Success",
	}

	return nil
}

// getNearbyEntities 使用AOI系统获取玩家附近的实体
func (s *SyncPlayerStateService) getNearbyEntities(playerID string) ([]*pb.EntityPosition, error) {
	// 检查玩家是否存在
	player, err := s.manager.GetPlayer(playerID)
	if err != nil {
		return nil, err
	}

	// 获取玩家位置
	x, y, z := player.GetPosition()

	// 使用AOI系统获取附近实体，视距离为100单位
	entities, err := s.manager.aoiManager.GetEntitiesByDistance(x, y, z, 100)
	if err != nil {
		return nil, err
	}

	// 转换为协议缓冲区实体格式
	var result []*pb.EntityPosition
	for _, entity := range entities {
		// 排除玩家自身
		if entity.GetID() == playerID {
			continue
		}

		// 将实体转换为PlayerState（假设所有实体都是PlayerState类型）
		if playerEntity, ok := entity.(*PlayerState); ok {
			playerEntity.mu.RLock()
			pbEntity := &pb.EntityPosition{
				EntityId:   playerEntity.ID,
				EntityType: "player", // 假设所有实体都是玩家
				Position:   playerEntity.Position,
				Rotation:   playerEntity.Rotation,
				Velocity:   playerEntity.Velocity,
			}
			playerEntity.mu.RUnlock()
			result = append(result, pbEntity)
		}
	}

	return result, nil
}

// BroadcastService 处理广播和组播消息
type BroadcastService struct {
	manager *PlayerStateManager
}

// NewBroadcastService 创建一个新的广播服务
func NewBroadcastService(manager *PlayerStateManager) *BroadcastService {
	return &BroadcastService{
		manager: manager,
	}
}

// BroadcastMessage 向所有玩家广播消息
func (s *BroadcastService) BroadcastMessage(ctx context.Context, req *pb.ChatRequest, resp *pb.ChatResponse) error {
	log.Printf("Broadcasting message from %s: %s", req.SenderName, req.Content)

	// 在实际实现中，这里应该向所有在线玩家发送消息
	s.manager.mu.RLock()
	playerCount := len(s.manager.players)
	s.manager.mu.RUnlock()

	resp.Delivered = true
	resp.MessageId = time.Now().UnixNano()
	resp.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
	resp.Header = &pb.MessageHeader{
		MsgId:      int32(pb.MessageType_CHAT_RESPONSE),
		Timestamp:  time.Now().UnixNano() / int64(time.Millisecond),
		SessionId:  req.Header.SessionId,
		Version:    req.Header.Version,
		ResultCode: 0,
		ResultMsg:  fmt.Sprintf("Message broadcast to %d players", playerCount),
	}

	return nil
}

// BroadcastToNearby 向玩家附近的其他玩家广播消息
func (s *BroadcastService) BroadcastToNearby(ctx context.Context, req *pb.ChatRequest, resp *pb.ChatResponse) error {
	log.Printf("Broadcasting message from %s to nearby players: %s", req.SenderName, req.Content)

	// 获取发送者玩家
	senderID := req.SenderId
	player, err := s.manager.GetPlayer(senderID)
	if err != nil {
		resp.Delivered = false
		resp.Header = &pb.MessageHeader{
			MsgId:      int32(pb.MessageType_CHAT_RESPONSE),
			Timestamp:  time.Now().UnixNano() / int64(time.Millisecond),
			SessionId:  req.Header.SessionId,
			Version:    req.Header.Version,
			ResultCode: 1,
			ResultMsg:  fmt.Sprintf("玩家不存在: %s", senderID),
		}
		return err
	}

	// 获取玩家位置
	x, y, z := player.GetPosition()

	// 获取100单位范围内的实体
	entities, err := s.manager.aoiManager.GetEntitiesByDistance(x, y, z, 100)
	if err != nil {
		resp.Delivered = false
		resp.Header = &pb.MessageHeader{
			MsgId:      int32(pb.MessageType_CHAT_RESPONSE),
			Timestamp:  time.Now().UnixNano() / int64(time.Millisecond),
			SessionId:  req.Header.SessionId,
			Version:    req.Header.Version,
			ResultCode: 1,
			ResultMsg:  fmt.Sprintf("获取附近实体失败: %v", err),
		}
		return err
	}

	// 计算附近玩家数量
	nearbyPlayers := 0
	for _, entity := range entities {
		if entity.GetID() != senderID && entity.GetTypeID() == 0 { // 排除自身，只计算玩家
			nearbyPlayers++
			// 在实际实现中，这里应该向附近每个玩家发送消息
		}
	}

	resp.Delivered = true
	resp.MessageId = time.Now().UnixNano()
	resp.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
	resp.Header = &pb.MessageHeader{
		MsgId:      int32(pb.MessageType_CHAT_RESPONSE),
		Timestamp:  time.Now().UnixNano() / int64(time.Millisecond),
		SessionId:  req.Header.SessionId,
		Version:    req.Header.Version,
		ResultCode: 0,
		ResultMsg:  fmt.Sprintf("Message broadcast to %d nearby players", nearbyPlayers),
	}

	return nil
}

// GroupcastMessage 向特定组的玩家发送消息
func (s *BroadcastService) GroupcastMessage(ctx context.Context, req *pb.ChatRequest, resp *pb.ChatResponse) error {
	log.Printf("Groupcasting message from %s to %s: %s",
		req.SenderName,
		req.ChatType.String(),
		req.Content)

	// 在实际实现中，这里应该根据聊天类型（私聊、队伍、公会等）发送消息给对应的玩家组
	targetCount := 0

	switch req.ChatType {
	case pb.ChatType_PRIVATE:
		// 私聊，只发给指定玩家
		if req.TargetId != "" {
			targetCount = 1
		}
	case pb.ChatType_TEAM:
		// 队伍聊天
		targetCount = 5 // 假设平均每个队伍5人
	case pb.ChatType_GUILD:
		// 公会聊天
		targetCount = 50 // 假设平均每个公会50人
	case pb.ChatType_CHANNEL:
		// 频道聊天
		targetCount = 100 // 假设平均每个频道100人
	}

	resp.Delivered = true
	resp.MessageId = time.Now().UnixNano()
	resp.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
	resp.Header = &pb.MessageHeader{
		MsgId:      int32(pb.MessageType_CHAT_RESPONSE),
		Timestamp:  time.Now().UnixNano() / int64(time.Millisecond),
		SessionId:  req.Header.SessionId,
		Version:    req.Header.Version,
		ResultCode: 0,
		ResultMsg:  fmt.Sprintf("Message delivered to %d players in %s", targetCount, req.ChatType.String()),
	}

	return nil
}
