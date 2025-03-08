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

	"github.com/google/uuid"
)

var (
	mode = flag.String("mode", "test", "运行模式 (test, registry)")
)

func main() {
	flag.Parse()

	switch *mode {
	case "test":
		runTest()
	case "registry":
		runRegistryService()
	default:
		fmt.Printf("未知模式: %s\n", *mode)
		fmt.Println("使用 -mode=test 或 -mode=registry")
		os.Exit(1)
	}
}

// 运行测试程序
func runTest() {
	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建内存注册表
	registry := rpc.NewInMemoryRegistry()

	// 创建负载均衡器
	loadBalancer := rpc.NewLoadBalancer(registry)

	// 为player服务设置轮询选择器
	loadBalancer.SetSelector("player-service", &rpc.RoundRobinSelector{})

	// 为聊天服务设置权重选择器
	loadBalancer.SetSelector("chat-service", &rpc.WeightedSelector{})

	// 启动服务监控
	go monitorService(ctx, registry, "player-service")

	// 模拟注册服务实例
	registerServices(ctx, registry)

	// 模拟客户端请求
	var wg sync.WaitGroup
	wg.Add(2)

	// 模拟对player服务的请求
	go func() {
		defer wg.Done()
		clientRequests(ctx, loadBalancer, "player-service", 10)
	}()

	// 模拟对chat服务的请求
	go func() {
		defer wg.Done()
		clientRequests(ctx, loadBalancer, "chat-service", 10)
	}()

	// 等待中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	select {
	case <-sigCh:
		log.Println("接收到中断信号，正在退出...")
		cancel()
	case <-ctx.Done():
	}

	wg.Wait()
	log.Println("程序已退出")
}

// 注册服务实例
func registerServices(ctx context.Context, registry rpc.ServiceRegistry) {
	// 注册三个player服务实例
	for i := 1; i <= 3; i++ {
		instance := &rpc.ServiceInstance{
			ID:       uuid.New().String(),
			Name:     "player-service",
			Endpoint: fmt.Sprintf("localhost:808%d", i),
			Tags:     map[string]string{"zone": fmt.Sprintf("zone-%d", (i-1)/2+1)},
			Status:   "active",
			Weight:   1,
		}

		if err := registry.Register(ctx, instance); err != nil {
			log.Printf("注册player服务实例失败: %v", err)
		} else {
			log.Printf("已注册player服务实例: %s 在 %s", instance.ID, instance.Endpoint)
		}
	}

	// 注册三个chat服务实例，使用不同权重
	for i := 1; i <= 3; i++ {
		instance := &rpc.ServiceInstance{
			ID:       uuid.New().String(),
			Name:     "chat-service",
			Endpoint: fmt.Sprintf("localhost:809%d", i),
			Tags:     map[string]string{"region": fmt.Sprintf("region-%d", (i-1)/2+1)},
			Status:   "active",
			Weight:   i * 2, // 权重依次为2, 4, 6
		}

		if err := registry.Register(ctx, instance); err != nil {
			log.Printf("注册chat服务实例失败: %v", err)
		} else {
			log.Printf("已注册chat服务实例: %s 在 %s (权重: %d)", instance.ID, instance.Endpoint, instance.Weight)
		}
	}

	// 延迟2秒后将一个player服务标记为下线
	go func() {
		time.Sleep(2 * time.Second)
		instances, _ := registry.GetInstances(ctx, "player-service")
		if len(instances) > 0 {
			log.Printf("将服务实例标记为下线: %s", instances[0].ID)
			registry.Deregister(ctx, instances[0].ID)
		}
	}()

	// 延迟4秒后添加一个新的player服务实例
	go func() {
		time.Sleep(4 * time.Second)
		instance := &rpc.ServiceInstance{
			ID:       uuid.New().String(),
			Name:     "player-service",
			Endpoint: "localhost:8084",
			Tags:     map[string]string{"zone": "zone-3"},
			Status:   "active",
			Weight:   1,
		}

		if err := registry.Register(ctx, instance); err != nil {
			log.Printf("注册新player服务实例失败: %v", err)
		} else {
			log.Printf("已注册新player服务实例: %s 在 %s", instance.ID, instance.Endpoint)
		}
	}()
}

// 监控服务变化
func monitorService(ctx context.Context, registry rpc.ServiceRegistry, serviceName string) {
	log.Printf("开始监控服务: %s", serviceName)

	ch, err := registry.WatchService(ctx, serviceName)
	if err != nil {
		log.Printf("监控服务失败: %v", err)
		return
	}

	for {
		select {
		case instances := <-ch:
			log.Printf("服务 %s 实例变化: 当前有 %d 个活跃实例", serviceName, len(instances))
			for _, inst := range instances {
				log.Printf("  - %s 在 %s", inst.ID, inst.Endpoint)
			}
		case <-ctx.Done():
			log.Printf("停止监控服务: %s", serviceName)
			return
		}
	}
}

// 模拟客户端请求
func clientRequests(ctx context.Context, lb *rpc.LoadBalancer, serviceName string, count int) {
	log.Printf("开始模拟客户端对 %s 的请求", serviceName)

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			instance, err := lb.GetInstance(ctx, serviceName)
			if err != nil {
				log.Printf("获取 %s 实例失败: %v", serviceName, err)
			} else {
				log.Printf("请求 %s: 选择实例 %s 在 %s", serviceName, instance.ID, instance.Endpoint)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	log.Printf("完成对 %s 的请求模拟", serviceName)
}

// 运行作为服务注册中心的服务
func runRegistryService() {
	registry := rpc.NewInMemoryRegistry()

	log.Println("服务注册中心已启动")

	// 创建上下文，用于优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// 启动所有服务的监控，打印注册情况
	go func() {
		// 定期打印所有注册的服务
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				services := []string{"player-service", "chat-service", "example-service"}
				for _, svc := range services {
					instances, _ := registry.GetInstances(ctx, svc)
					if len(instances) > 0 {
						log.Printf("服务 %s: %d 个实例", svc, len(instances))
						for _, inst := range instances {
							log.Printf("  - %s 在 %s (权重: %d, 状态: %s)",
								inst.ID, inst.Endpoint, inst.Weight, inst.Status)
						}
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	<-sigCh
	log.Println("正在关闭服务注册中心...")
}
