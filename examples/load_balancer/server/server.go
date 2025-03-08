package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"mmo-server/pkg/rpc"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

var (
	serverID     = flag.String("id", "", "服务器ID (如果为空则自动生成)")
	serviceName  = flag.String("service", "example-service", "服务名称")
	port         = flag.Int("port", 8080, "服务端口")
	registryAddr = flag.String("registry", "localhost:8500", "服务注册中心地址")
	weight       = flag.Int("weight", 1, "服务权重")
	zone         = flag.String("zone", "default", "服务区域")
)

// 简单的RPC服务实现
type ExampleService struct{}

func (s *ExampleService) Hello(ctx context.Context, name string) (string, error) {
	return fmt.Sprintf("Hello, %s! From server %s", name, *serverID), nil
}

func main() {
	flag.Parse()

	// 如果没有指定ID，则生成一个随机ID
	if *serverID == "" {
		*serverID = uuid.New().String()[:8]
	}

	// 创建服务实例
	instance := &rpc.ServiceInstance{
		ID:       *serverID,
		Name:     *serviceName,
		Endpoint: fmt.Sprintf("localhost:%d", *port),
		Tags:     map[string]string{"zone": *zone},
		Status:   "active",
		Weight:   *weight,
	}

	// 创建内存注册表 (在实际环境中，这里应该使用集中式的服务注册中心)
	registry := rpc.NewInMemoryRegistry()

	// 注册信号处理，确保优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalChan
		log.Printf("接收到信号: %v，准备关闭服务", sig)

		// 注销服务
		if err := registry.Deregister(ctx, instance.ID); err != nil {
			log.Printf("注销服务失败: %v", err)
		}

		cancel()
	}()

	// 创建gRPC服务器
	server := grpc.NewServer()

	// 启动网络监听
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("监听端口失败: %v", err)
	}

	// 注册服务
	log.Printf("注册服务: %s, ID: %s, 地址: %s", instance.Name, instance.ID, instance.Endpoint)
	if err := registry.Register(ctx, instance); err != nil {
		log.Fatalf("注册服务失败: %v", err)
	}

	// 启动服务健康检查
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 更新服务状态
				instance.Status = "active"
				if err := registry.Register(ctx, instance); err != nil {
					log.Printf("更新服务状态失败: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 创建并注册ExampleService
	exampleService := &ExampleService{}
	// 在实际应用中，这里应该注册gRPC服务
	// 例如: pb.RegisterExampleServiceServer(server, exampleService)
	_ = exampleService // 避免未使用警告

	// 这里应该注册gRPC服务，但我们只是模拟这个过程
	log.Printf("服务 %s 已启动，监听端口: %d", instance.Name, *port)

	// 启动控制台，接收简单命令
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				fmt.Println("\n可用命令:")
				fmt.Println("  status - 显示服务状态")
				fmt.Println("  down - 将服务标记为下线")
				fmt.Println("  up - 将服务标记为上线")
				fmt.Println("  weight <值> - 更改服务权重")
				fmt.Println("  exit - 退出程序")

				var cmd string
				fmt.Print("> ")
				fmt.Scanln(&cmd)

				switch cmd {
				case "status":
					fmt.Printf("服务状态: %s, 权重: %d\n", instance.Status, instance.Weight)
				case "down":
					instance.Status = "down"
					if err := registry.Register(ctx, instance); err != nil {
						fmt.Printf("更新状态失败: %v\n", err)
					} else {
						fmt.Println("服务已标记为下线")
					}
				case "up":
					instance.Status = "active"
					if err := registry.Register(ctx, instance); err != nil {
						fmt.Printf("更新状态失败: %v\n", err)
					} else {
						fmt.Println("服务已标记为上线")
					}
				case "exit":
					signalChan <- syscall.SIGTERM
					return
				default:
					if len(cmd) > 7 && cmd[:7] == "weight " {
						w, err := strconv.Atoi(cmd[7:])
						if err != nil {
							fmt.Println("无效的权重值")
							continue
						}
						instance.Weight = w
						if err := registry.Register(ctx, instance); err != nil {
							fmt.Printf("更新权重失败: %v\n", err)
						} else {
							fmt.Printf("服务权重已更新为: %d\n", w)
						}
					}
				}
			}
		}
	}()

	// 启动gRPC服务
	if err := server.Serve(listener); err != nil && err != grpc.ErrServerStopped {
		log.Fatalf("启动gRPC服务失败: %v", err)
	}

	// 等待上下文结束
	<-ctx.Done()
	log.Println("服务正在关闭...")

	// 优雅关闭服务器
	server.GracefulStop()
	log.Println("服务已关闭")
}
