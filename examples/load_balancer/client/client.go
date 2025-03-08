package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"mmo-server/pkg/rpc"
)

var (
	serviceName   = flag.String("service", "example-service", "要调用的服务名称")
	requestCount  = flag.Int("count", 10, "发送请求的数量")
	interval      = flag.Duration("interval", 1*time.Second, "请求间隔")
	balanceType   = flag.String("lb", "random", "负载均衡类型: random, round-robin, weighted")
	monitorChange = flag.Bool("monitor", true, "是否监控服务变化")
)

// 模拟RPC请求
func doRequest(ctx context.Context, client rpc.RPCClient, id int) {
	// 由于RPCClient接口没有GetEndpoint方法，我们直接记录请求信息
	log.Printf("发送请求 #%d", id)

	// 在实际应用中，这里应该调用实际的gRPC接口
	// 例如: response, err := client.Call(ctx, "Hello", "World")

	// 模拟处理时间
	time.Sleep(100 * time.Millisecond)

	log.Printf("请求 #%d 完成", id)
}

func main() {
	flag.Parse()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建内存注册表
	registry := rpc.NewInMemoryRegistry()

	// 创建负载均衡器
	lb := rpc.NewLoadBalancer(registry)

	// 根据命令行参数设置负载均衡类型
	switch *balanceType {
	case "round-robin":
		lb.SetSelector(*serviceName, &rpc.RoundRobinSelector{})
		log.Println("使用轮询负载均衡")
	case "weighted":
		lb.SetSelector(*serviceName, &rpc.WeightedSelector{})
		log.Println("使用加权负载均衡")
	default:
		// 默认使用随机选择器
		lb.SetSelector(*serviceName, &rpc.RandomSelector{})
		log.Println("使用随机负载均衡")
	}

	// 注册信号处理
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChan
		log.Println("收到中断信号，正在退出...")
		cancel()
	}()

	// 监控服务变化
	if *monitorChange {
		go func() {
			log.Printf("开始监控服务: %s", *serviceName)

			watch, err := registry.WatchService(ctx, *serviceName)
			if err != nil {
				log.Printf("监控服务失败: %v", err)
				return
			}

			for {
				select {
				case instances, ok := <-watch:
					if !ok {
						return
					}
					log.Printf("服务 %s 变化: 当前有 %d 个活跃实例", *serviceName, len(instances))
					for _, inst := range instances {
						log.Printf("  - %s 在 %s (权重: %d)", inst.ID, inst.Endpoint, inst.Weight)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// 执行请求
	var wg sync.WaitGroup

	for i := 1; i <= *requestCount; i++ {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			log.Println("操作已取消，停止发送请求")
			break
		default:
		}

		wg.Add(1)
		go func(reqID int) {
			defer wg.Done()

			// 使用负载均衡器获取服务实例
			instance, err := lb.GetInstance(ctx, *serviceName)
			if err != nil {
				log.Printf("获取服务实例失败: %v", err)
				return
			}

			log.Printf("请求 #%d 使用实例: %s 在 %s", reqID, instance.ID, instance.Endpoint)

			// 使用负载均衡器获取客户端
			client, err := lb.GetClient(ctx, *serviceName)
			if err != nil {
				log.Printf("获取客户端失败: %v", err)
				return
			}

			// 执行请求
			doRequest(ctx, client, reqID)
		}(i)

		// 按指定间隔发送请求
		time.Sleep(*interval)
	}

	// 等待所有请求完成
	wg.Wait()
	log.Println("所有请求已完成")

	// 通过上下文取消监控
	cancel()

	// 等待一段时间，确保所有goroutine都有机会退出
	time.Sleep(100 * time.Millisecond)
	log.Println("客户端已退出")
}
