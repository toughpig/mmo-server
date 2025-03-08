package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"mmo-server/pkg/rpc"
	syncpkg "mmo-server/pkg/sync"
	pb "mmo-server/proto_define"
)

const (
	ModeServer  = "server"
	ModeClient  = "client"
	ModeExample = "example"
)

func main() {
	// 解析命令行参数
	mode := flag.String("mode", "example", "运行模式：server, client 或 example")
	endpoint := flag.String("endpoint", "/tmp/mmo-sync-test.sock", "RPC端点路径")
	playerID := flag.String("player", "player1", "玩家ID (仅客户端模式)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("启动同步测试，模式：%s，端点：%s", *mode, *endpoint)

	// 处理信号以优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	switch *mode {
	case ModeServer:
		runServer(*endpoint, sigChan)
	case ModeClient:
		runClient(*endpoint, *playerID, sigChan)
	case ModeExample:
		runExample()
	default:
		fmt.Printf("未知模式：%s\n", *mode)
		flag.Usage()
		os.Exit(1)
	}

	log.Println("同步测试完成")
}

// runServer 运行服务器模式
func runServer(endpoint string, sigChan chan os.Signal) {
	// 删除可能存在的旧socket文件
	os.Remove(endpoint)

	// 创建玩家状态管理器
	stateManager := syncpkg.NewPlayerStateManager()

	// 创建服务
	syncService := syncpkg.NewSyncPlayerStateService(stateManager)
	broadcastService := syncpkg.NewBroadcastService(stateManager)

	// 创建并启动RPC服务器
	server := rpc.NewShmIPCServer(endpoint)

	// 注册服务
	err := server.Register(syncService)
	if err != nil {
		log.Fatalf("无法注册同步服务：%v", err)
	}

	err = server.Register(broadcastService)
	if err != nil {
		log.Fatalf("无法注册广播服务：%v", err)
	}

	// 启动服务器
	err = server.Start()
	if err != nil {
		log.Fatalf("无法启动RPC服务器：%v", err)
	}
	defer server.Stop()

	log.Printf("服务器已启动，等待客户端连接...")

	// 注册回调以记录状态更新
	stateManager.RegisterCallback("log", func(playerID string, state *syncpkg.PlayerState) error {
		log.Printf("玩家 %s 状态更新：位置=(%f,%f,%f), 速度=(%f,%f,%f), 旋转=%f",
			playerID,
			state.Position.X, state.Position.Y, state.Position.Z,
			state.Velocity.X, state.Velocity.Y, state.Velocity.Z,
			state.Rotation)
		return nil
	})

	// 等待信号退出
	<-sigChan
	log.Println("正在停止服务器...")
}

// runClient 运行客户端模式
func runClient(endpoint string, playerID string, sigChan chan os.Signal) {
	// 创建RPC客户端
	client, err := rpc.NewShmIPCClient(endpoint)
	if err != nil {
		log.Fatalf("无法创建RPC客户端：%v", err)
	}
	defer client.Close()

	log.Printf("客户端已连接，玩家ID：%s", playerID)

	// 周期性更新玩家位置
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// 保持位置更新的状态
	x, y, z := 0.0, 0.0, 0.0
	rotation := 0.0

	for {
		select {
		case <-ticker.C:
			// 更新位置
			x += 1.0
			z += 0.5
			rotation += 15.0
			// 将旋转限制在0-360度范围内
			for rotation >= 360.0 {
				rotation -= 360.0
			}

			// 创建位置同步请求
			req := &pb.PositionSyncRequest{
				Header: &pb.MessageHeader{
					MsgId:     int32(pb.MessageType_POSITION_SYNC_REQUEST),
					Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
					SessionId: fmt.Sprintf("session-%s", playerID),
					Version:   1,
				},
				EntityId: playerID,
				Position: &pb.Position{
					X: float32(x),
					Y: float32(y),
					Z: float32(z),
				},
				Velocity: &pb.Vector3{
					X: 1.0,
					Y: 0.0,
					Z: 0.5,
				},
				Rotation:  float32(rotation),
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			}

			// 准备响应对象
			resp := &pb.PositionSyncResponse{}

			// 调用同步服务
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err = client.Call(ctx, "SyncPlayerStateService.SyncPosition", req, resp)
			cancel()

			if err != nil {
				log.Printf("同步位置失败：%v", err)
				continue
			}

			log.Printf("位置同步成功，附近实体数量：%d", len(resp.NearbyEntities))

			// 打印附近实体信息
			for i, entity := range resp.NearbyEntities {
				log.Printf("附近实体 %d: ID=%s, 类型=%s, 位置=(%f,%f,%f)",
					i+1, entity.EntityId, entity.EntityType,
					entity.Position.X, entity.Position.Y, entity.Position.Z)
			}

			// 也发送一条广播消息
			// 使用整数取模运算来判断是否需要发送消息
			xInt := int(x)
			if xInt%10 == 0 { // 每10个整数位置发送一次
				chatReq := &pb.ChatRequest{
					Header: &pb.MessageHeader{
						MsgId:     int32(pb.MessageType_CHAT_REQUEST),
						Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
						SessionId: fmt.Sprintf("session-%s", playerID),
						Version:   1,
					},
					SenderId:   playerID,
					SenderName: fmt.Sprintf("Player_%s", playerID),
					Content:    fmt.Sprintf("Hello from position (%f,%f,%f)!", x, y, z),
					ChatType:   pb.ChatType_WORLD,
				}

				chatResp := &pb.ChatResponse{}

				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
				err = client.Call(ctx, "BroadcastService.BroadcastMessage", chatReq, chatResp)
				cancel()

				if err != nil {
					log.Printf("发送广播消息失败：%v", err)
				} else {
					log.Printf("广播消息发送成功，消息ID：%d", chatResp.MessageId)
				}
			}

		case <-sigChan:
			log.Println("客户端正在退出...")
			return
		}
	}
}

// runExample 运行示例模式（服务器和客户端在同一个进程中）
func runExample() {
	log.Println("启动完整示例...")

	socketPath := "/tmp/mmo-sync-example.sock"
	os.Remove(socketPath)

	// 创建状态管理器和服务
	stateManager := syncpkg.NewPlayerStateManager()
	syncService := syncpkg.NewSyncPlayerStateService(stateManager)
	broadcastService := syncpkg.NewBroadcastService(stateManager)

	// 创建并启动RPC服务器
	server := rpc.NewShmIPCServer(socketPath)

	// 注册服务
	err := server.Register(syncService)
	if err != nil {
		log.Fatalf("无法注册同步服务：%v", err)
	}

	err = server.Register(broadcastService)
	if err != nil {
		log.Fatalf("无法注册广播服务：%v", err)
	}

	// 启动服务器
	err = server.Start()
	if err != nil {
		log.Fatalf("无法启动RPC服务器：%v", err)
	}
	defer server.Stop()

	log.Println("服务器已启动，创建测试玩家...")

	// 注册回调以记录状态更新
	stateManager.RegisterCallback("log", func(playerID string, state *syncpkg.PlayerState) error {
		log.Printf("玩家 %s 状态更新：位置=(%f,%f,%f), 旋转=%f",
			playerID,
			state.Position.X, state.Position.Y, state.Position.Z,
			state.Rotation)
		return nil
	})

	// 预先注册几个玩家
	stateManager.RegisterPlayer("player1")
	stateManager.RegisterPlayer("player2")
	stateManager.RegisterPlayer("player3")

	// 创建两个客户端，模拟多个玩家
	clients := make([]*rpc.ShmIPCClient, 2)
	for i := range clients {
		client, err := rpc.NewShmIPCClient(socketPath)
		if err != nil {
			log.Fatalf("无法创建RPC客户端 %d：%v", i+1, err)
		}
		defer client.Close()
		clients[i] = client
	}

	// 让两个客户端互相订阅对方的状态
	stateManager.SubscribeToPlayer("player1", "player2")
	stateManager.SubscribeToPlayer("player2", "player1")

	log.Println("开始模拟两个玩家移动...")

	// 使用WaitGroup来等待所有客户端完成
	var wg sync.WaitGroup
	wg.Add(2)

	// 启动两个玩家的模拟
	for i, client := range clients {
		go func(idx int, c *rpc.ShmIPCClient) {
			defer wg.Done()
			playerID := fmt.Sprintf("player%d", idx+1)

			// 每个玩家移动5次
			for j := 0; j < 5; j++ {
				// 计算旋转角度，保持在0-360度范围内
				rotationDeg := 45.0 * float64(j)
				for rotationDeg >= 360.0 {
					rotationDeg -= 360.0
				}

				// 创建位置同步请求
				req := &pb.PositionSyncRequest{
					Header: &pb.MessageHeader{
						MsgId:     int32(pb.MessageType_POSITION_SYNC_REQUEST),
						Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
						SessionId: fmt.Sprintf("session-%s", playerID),
						Version:   1,
					},
					EntityId: playerID,
					Position: &pb.Position{
						X: float32(10.0*float64(idx+1) + float64(j)*2.0),
						Y: 0.0,
						Z: float32(5.0*float64(idx+1) + float64(j)*1.0),
					},
					Velocity: &pb.Vector3{
						X: 1.0,
						Y: 0.0,
						Z: 0.5,
					},
					Rotation:  float32(rotationDeg),
					Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
				}

				resp := &pb.PositionSyncResponse{}

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				err := c.Call(ctx, "SyncPlayerStateService.SyncPosition", req, resp)
				cancel()

				if err != nil {
					log.Printf("玩家 %s 同步位置失败：%v", playerID, err)
				} else {
					log.Printf("玩家 %s 位置同步成功，附近实体数量：%d", playerID, len(resp.NearbyEntities))

					// 显示附近实体
					for k, entity := range resp.NearbyEntities {
						log.Printf("玩家 %s 的附近实体 %d: ID=%s, 位置=(%f,%f,%f)",
							playerID, k+1, entity.EntityId,
							entity.Position.X, entity.Position.Y, entity.Position.Z)
					}
				}

				// 每秒更新一次
				time.Sleep(1 * time.Second)
			}

			// 发送一条广播消息
			chatReq := &pb.ChatRequest{
				Header: &pb.MessageHeader{
					MsgId:     int32(pb.MessageType_CHAT_REQUEST),
					Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
					SessionId: fmt.Sprintf("session-%s", playerID),
					Version:   1,
				},
				SenderId:   playerID,
				SenderName: fmt.Sprintf("Player_%s", playerID),
				Content:    fmt.Sprintf("玩家 %s 的测试消息", playerID),
				ChatType:   pb.ChatType_WORLD,
			}

			chatResp := &pb.ChatResponse{}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := c.Call(ctx, "BroadcastService.BroadcastMessage", chatReq, chatResp)
			cancel()

			if err != nil {
				log.Printf("玩家 %s 发送广播消息失败：%v", playerID, err)
			} else {
				log.Printf("玩家 %s 广播消息发送成功，消息ID：%d", playerID, chatResp.MessageId)
			}
		}(i, client)
	}

	// 等待所有玩家完成
	wg.Wait()
	log.Println("示例完成")
}
