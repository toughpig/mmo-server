package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	stdsync "sync"
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
	// 定义命令行参数
	mode := flag.String("mode", "example", "运行模式: 'server', 'client', 'example'")
	endpoint := flag.String("endpoint", "/tmp/mmo-sync-test.sock", "IPC端点路径（Unix socket）")
	playerCount := flag.Int("players", 5, "客户端模式中创建的玩家数量")
	updateCount := flag.Int("updates", 20, "每个玩家的状态更新次数")
	updateInterval := flag.Int("interval", 100, "状态更新间隔，单位毫秒")
	flag.Parse()

	// 设置日志记录
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("启动同步测试，模式：%s，端点：%s", *mode, *endpoint)

	// 处理信号，实现优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	switch *mode {
	case "server":
		// 创建并启动RPC服务器
		server := rpc.NewShmIPCServer(*endpoint)

		// 创建玩家状态管理器
		playerStateManager := syncpkg.NewPlayerStateManager()

		// 创建同步服务
		syncService := syncpkg.NewSyncPlayerStateService(playerStateManager)
		broadcastService := syncpkg.NewBroadcastService(playerStateManager)

		// 注册服务
		err := server.Register(syncService)
		if err != nil {
			log.Fatalf("无法注册同步服务：%v", err)
		}

		err = server.Register(broadcastService)
		if err != nil {
			log.Fatalf("无法注册广播服务：%v", err)
		}

		// 删除已存在的socket文件
		if _, err := os.Stat(*endpoint); err == nil {
			log.Printf("删除已存在的socket文件：%s", *endpoint)
			if err := os.Remove(*endpoint); err != nil {
				log.Fatalf("无法删除已存在的socket文件：%v", err)
			}
		}

		// 启动服务器
		err = server.Start()
		if err != nil {
			log.Fatalf("无法启动服务器：%v", err)
		}
		log.Printf("服务器已启动，端点：%s", *endpoint)
		log.Println("按Ctrl+C停止服务器")

		// 等待信号以停止
		<-sigChan
		log.Println("正在停止服务器...")
		server.Stop()

	case "client":
		// 创建RPC客户端
		client, err := rpc.NewShmIPCClient(*endpoint)
		if err != nil {
			log.Fatalf("无法创建客户端：%v", err)
		}
		defer client.Close()

		// 创建一组测试玩家
		var wg stdsync.WaitGroup
		wg.Add(*playerCount)

		for p := 0; p < *playerCount; p++ {
			go func(playerID int) {
				defer wg.Done()
				playerName := fmt.Sprintf("test-player-%d", playerID)

				// 每个玩家同步几次位置
				for i := 0; i < *updateCount; i++ {
					posX := float32(playerID*10) + float32(i)
					posY := float32(10)
					posZ := float32(playerID*10) - float32(i)

					// 创建位置同步请求
					syncReq := &pb.PositionSyncRequest{
						Header: &pb.MessageHeader{
							MsgId:     int32(pb.MessageType_POSITION_SYNC_REQUEST),
							Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
							SessionId: fmt.Sprintf("session-%s", playerName),
							Version:   1,
						},
						EntityId: playerName,
						Position: &pb.Position{
							X: posX,
							Y: posY,
							Z: posZ,
						},
						Velocity: &pb.Vector3{
							X: 1,
							Y: 0,
							Z: 1,
						},
						Rotation:  float32(i * 10),
						Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
					}
					syncResp := &pb.PositionSyncResponse{}

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					err := client.Call(ctx, "SyncPlayerStateService.SyncPosition", syncReq, syncResp)
					cancel()

					if err != nil {
						log.Printf("玩家%s位置同步失败：%v", playerName, err)
					} else {
						log.Printf("玩家%s位置同步成功，附近实体数量：%d", playerName, len(syncResp.NearbyEntities))
					}

					time.Sleep(time.Duration(*updateInterval) * time.Millisecond)
				}

				// 发送一条广播消息
				chatReq := &pb.ChatRequest{
					Header: &pb.MessageHeader{
						MsgId:     int32(pb.MessageType_CHAT_REQUEST),
						Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
						SessionId: fmt.Sprintf("session-%s", playerName),
						Version:   1,
					},
					SenderId:   playerName,
					SenderName: fmt.Sprintf("Player_%s", playerName),
					Content:    fmt.Sprintf("来自%s的测试消息", playerName),
					ChatType:   pb.ChatType_WORLD,
				}
				chatResp := &pb.ChatResponse{}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err = client.Call(ctx, "BroadcastService.BroadcastMessage", chatReq, chatResp)
				cancel()

				if err != nil {
					log.Printf("玩家%s广播消息失败：%v", playerName, err)
				} else {
					log.Printf("玩家%s广播消息成功，消息ID：%d", playerName, chatResp.MessageId)
				}
			}(p)
		}

		// 等待所有玩家完成测试
		wg.Wait()
		log.Println("客户端测试完成")

	case "example":
		runExampleTest()
	default:
		log.Fatalf("未知模式：%s", *mode)
	}
}

// 运行完整示例测试
func runExampleTest() {
	log.Println("启动完整示例...")

	// 创建玩家状态管理器
	playerStateManager := syncpkg.NewPlayerStateManager()

	// 创建同步服务
	syncService := syncpkg.NewSyncPlayerStateService(playerStateManager)
	broadcastService := syncpkg.NewBroadcastService(playerStateManager)

	// 创建并启动服务器
	socketPath := "/tmp/mmo-sync-example.sock"
	if _, err := os.Stat(socketPath); err == nil {
		os.Remove(socketPath)
	}

	server := rpc.NewShmIPCServer(socketPath)
	server.Register(syncService)
	server.Register(broadcastService)
	server.Start()

	// 在服务器启动后创建测试玩家
	log.Println("服务器已启动，创建测试玩家...")

	// 注册几个玩家
	player1 := "player1"
	player2 := "player2"
	player3 := "player3"
	playerStateManager.RegisterPlayer(player1)
	playerStateManager.RegisterPlayer(player2)
	playerStateManager.RegisterPlayer(player3)

	// 让玩家2订阅玩家1
	playerStateManager.SubscribeToPlayer(player2, player1)

	// 创建RPC客户端
	time.Sleep(500 * time.Millisecond) // 给服务器一些启动时间

	// 尝试创建5个客户端，验证并发访问
	var wg stdsync.WaitGroup
	wg.Add(5)

	for i := 1; i <= 5; i++ {
		go func(clientID int) {
			defer wg.Done()
			client, err := rpc.NewShmIPCClient(socketPath)
			if err != nil {
				log.Printf("无法创建RPC客户端 %d：%v", clientID, err)
				return
			}
			defer client.Close()

			// 发送位置更新
			playerID := fmt.Sprintf("player%d", clientID%3+1) // 使用player1, player2, player3
			syncReq := &pb.PositionSyncRequest{
				Header: &pb.MessageHeader{
					MsgId:     int32(pb.MessageType_POSITION_SYNC_REQUEST),
					Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
					SessionId: fmt.Sprintf("session-%s", playerID),
					Version:   1,
				},
				EntityId: playerID,
				Position: &pb.Position{
					X: float32(clientID * 100),
					Y: float32(clientID * 10),
					Z: float32(clientID * 50),
				},
				Velocity: &pb.Vector3{
					X: float32(clientID),
					Y: 0,
					Z: float32(clientID),
				},
				Rotation:  float32(clientID * 45),
				Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			}
			syncResp := &pb.PositionSyncResponse{}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = client.Call(ctx, "SyncPlayerStateService.SyncPosition", syncReq, syncResp)
			cancel()

			if err != nil {
				log.Printf("客户端%d位置同步失败：%v", clientID, err)
			} else {
				log.Printf("客户端%d位置同步成功，附近实体数量：%d", clientID, len(syncResp.NearbyEntities))
			}

			// 发送广播消息
			if clientID == 1 {
				chatReq := &pb.ChatRequest{
					Header: &pb.MessageHeader{
						MsgId:     int32(pb.MessageType_CHAT_REQUEST),
						Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
						SessionId: fmt.Sprintf("session-%s", playerID),
						Version:   1,
					},
					SenderId:   playerID,
					SenderName: fmt.Sprintf("TestPlayer%d", clientID),
					Content:    "Hello, World!",
					ChatType:   pb.ChatType_WORLD,
				}
				chatResp := &pb.ChatResponse{}

				ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
				err = client.Call(ctx, "BroadcastService.BroadcastMessage", chatReq, chatResp)
				cancel()

				if err != nil {
					log.Printf("广播消息失败：%v", err)
				} else {
					log.Printf("广播消息成功，消息ID：%d", chatResp.MessageId)
				}

				// 发送组播消息
				teamReq := &pb.ChatRequest{
					Header: &pb.MessageHeader{
						MsgId:     int32(pb.MessageType_CHAT_REQUEST),
						Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
						SessionId: fmt.Sprintf("session-%s", playerID),
						Version:   1,
					},
					SenderId:   playerID,
					SenderName: fmt.Sprintf("TestPlayer%d", clientID),
					Content:    "Team message",
					ChatType:   pb.ChatType_TEAM,
					TargetId:   "TEAM", // 目标组ID
				}
				teamResp := &pb.ChatResponse{}

				ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
				err = client.Call(ctx, "BroadcastService.GroupcastMessage", teamReq, teamResp)
				cancel()

				if err != nil {
					log.Printf("组播消息失败：%v", err)
				} else {
					log.Printf("组播消息成功，消息ID：%d", teamResp.MessageId)
				}
			}
		}(i)
	}

	// 等待客户端完成
	wg.Wait()

	// 获取玩家状态
	for i, player := range []string{player1, player2, player3} {
		state, err := playerStateManager.GetPlayer(player)
		if err == nil && state != nil {
			log.Printf("玩家%d当前位置: (%f,%f,%f)", i+1,
				state.Position.X, state.Position.Y, state.Position.Z)
		}
	}

	// 清理
	playerStateManager.UnregisterPlayer(player1)
	playerStateManager.UnregisterPlayer(player2)
	playerStateManager.UnregisterPlayer(player3)

	// 停止服务器
	log.Println("测试完成，停止服务器...")
	server.Stop()
}
