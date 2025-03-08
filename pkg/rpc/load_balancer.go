package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// ServiceInstance 表示一个服务实例
type ServiceInstance struct {
	ID       string            // 实例唯一标识
	Name     string            // 服务名称
	Endpoint string            // 连接地址
	Tags     map[string]string // 标签，用于筛选
	Weight   int               // 权重，用于加权轮询
	Status   string            // 状态（active, draining, down）
}

// ServiceRegistry 服务注册和发现接口
type ServiceRegistry interface {
	// Register 注册服务实例
	Register(ctx context.Context, instance *ServiceInstance) error

	// Deregister 注销服务实例
	Deregister(ctx context.Context, instanceID string) error

	// GetInstances 获取指定服务的实例列表
	GetInstances(ctx context.Context, serviceName string) ([]*ServiceInstance, error)

	// WatchService 监听服务变化
	WatchService(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error)
}

// InstanceSelector 实例选择器接口，负载均衡器使用
type InstanceSelector interface {
	// Select 选择一个服务实例
	Select(instances []*ServiceInstance) (*ServiceInstance, error)
}

// LoadBalancer 负载均衡器实现
type LoadBalancer struct {
	registry  ServiceRegistry
	selectors map[string]InstanceSelector
	mu        sync.RWMutex
}

// NewLoadBalancer 创建负载均衡器
func NewLoadBalancer(registry ServiceRegistry) *LoadBalancer {
	return &LoadBalancer{
		registry:  registry,
		selectors: make(map[string]InstanceSelector),
	}
}

// SetSelector 为服务设置选择器
func (lb *LoadBalancer) SetSelector(serviceName string, selector InstanceSelector) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.selectors[serviceName] = selector
}

// GetInstance 获取服务的一个实例
func (lb *LoadBalancer) GetInstance(ctx context.Context, serviceName string) (*ServiceInstance, error) {
	// 获取服务实例列表
	instances, err := lb.registry.GetInstances(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances found for service: %s", serviceName)
	}

	// 获取选择器
	lb.mu.RLock()
	selector, ok := lb.selectors[serviceName]
	if !ok {
		// 默认使用随机选择器
		selector = &RandomSelector{}
	}
	lb.mu.RUnlock()

	// 选择一个实例
	return selector.Select(instances)
}

// GetClient 获取服务的客户端
func (lb *LoadBalancer) GetClient(ctx context.Context, serviceName string) (RPCClient, error) {
	instance, err := lb.GetInstance(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	// 使用工厂创建客户端
	return NewRPCClient(instance.Endpoint, TransportGRPC)
}

// 以下是几种不同的选择器实现

// RandomSelector 随机选择器
type RandomSelector struct{}

func (s *RandomSelector) Select(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("no instances available")
	}

	idx := rand.Intn(len(instances))
	return instances[idx], nil
}

// RoundRobinSelector 轮询选择器
type RoundRobinSelector struct {
	nextIdx int
	mu      sync.Mutex
}

func (s *RoundRobinSelector) Select(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("no instances available")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.nextIdx % len(instances)
	s.nextIdx++

	return instances[idx], nil
}

// WeightedSelector 加权选择器
type WeightedSelector struct{}

func (s *WeightedSelector) Select(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, errors.New("no instances available")
	}

	// 计算总权重
	totalWeight := 0
	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1 // 最小权重为1
		}
		totalWeight += w
	}

	// 随机选择
	target := rand.Intn(totalWeight)
	current := 0

	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}

		current += w
		if current > target {
			return inst, nil
		}
	}

	// 理论上不应该到这里，但是以防万一
	return instances[0], nil
}

// InMemoryRegistry 内存实现的服务注册表
type InMemoryRegistry struct {
	services    map[string][]*ServiceInstance
	watches     map[string][]chan []*ServiceInstance
	mu          sync.RWMutex
	cleanupTime time.Duration
}

// NewInMemoryRegistry 创建内存服务注册表
func NewInMemoryRegistry() *InMemoryRegistry {
	reg := &InMemoryRegistry{
		services:    make(map[string][]*ServiceInstance),
		watches:     make(map[string][]chan []*ServiceInstance),
		cleanupTime: 30 * time.Second,
	}

	// 启动定期清理下线实例的协程
	go reg.cleanup()

	return reg
}

// 定期清理下线实例
func (r *InMemoryRegistry) cleanup() {
	ticker := time.NewTicker(r.cleanupTime)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		for name, instances := range r.services {
			active := make([]*ServiceInstance, 0, len(instances))
			changed := false

			for _, inst := range instances {
				if inst.Status != "down" {
					active = append(active, inst)
				} else {
					changed = true
				}
			}

			if changed {
				r.services[name] = active
				r.notifyWatchers(name, active)
			}
		}
		r.mu.Unlock()
	}
}

// 通知监听器服务变化
func (r *InMemoryRegistry) notifyWatchers(serviceName string, instances []*ServiceInstance) {
	watches, ok := r.watches[serviceName]
	if !ok {
		return
	}

	for _, ch := range watches {
		// 使用非阻塞发送，避免死锁
		select {
		case ch <- instances:
		default:
			log.Printf("Watcher for service %s is not ready to receive", serviceName)
		}
	}
}

// Register 注册服务实例
func (r *InMemoryRegistry) Register(ctx context.Context, instance *ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查实例是否已存在
	instances, ok := r.services[instance.Name]
	if !ok {
		r.services[instance.Name] = []*ServiceInstance{instance}
		r.notifyWatchers(instance.Name, []*ServiceInstance{instance})
		return nil
	}

	// 检查ID是否重复
	for i, inst := range instances {
		if inst.ID == instance.ID {
			// 替换已存在的实例
			instances[i] = instance
			r.services[instance.Name] = instances
			r.notifyWatchers(instance.Name, instances)
			return nil
		}
	}

	// 添加新实例
	instances = append(instances, instance)
	r.services[instance.Name] = instances
	r.notifyWatchers(instance.Name, instances)

	return nil
}

// Deregister 注销服务实例
func (r *InMemoryRegistry) Deregister(ctx context.Context, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 查找实例
	for svcName, instances := range r.services {
		for i, inst := range instances {
			if inst.ID == instanceID {
				// 标记为下线状态，而不是直接删除
				r.services[svcName][i].Status = "down"
				r.notifyWatchers(svcName, r.services[svcName])
				return nil
			}
		}
	}

	return fmt.Errorf("instance %s not found", instanceID)
}

// GetInstances 获取服务实例列表
func (r *InMemoryRegistry) GetInstances(ctx context.Context, serviceName string) ([]*ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances, ok := r.services[serviceName]
	if !ok {
		return nil, nil
	}

	// 只返回活跃实例
	active := make([]*ServiceInstance, 0, len(instances))
	for _, inst := range instances {
		if inst.Status != "down" {
			active = append(active, inst)
		}
	}

	return active, nil
}

// WatchService 监听服务变化
func (r *InMemoryRegistry) WatchService(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 创建监听通道
	ch := make(chan []*ServiceInstance, 1)

	// 添加到监听列表
	watches, ok := r.watches[serviceName]
	if !ok {
		r.watches[serviceName] = []chan []*ServiceInstance{ch}
	} else {
		r.watches[serviceName] = append(watches, ch)
	}

	// 立即发送当前实例列表
	instances, ok := r.services[serviceName]
	if ok {
		// 只发送活跃实例
		active := make([]*ServiceInstance, 0, len(instances))
		for _, inst := range instances {
			if inst.Status != "down" {
				active = append(active, inst)
			}
		}

		ch <- active
	} else {
		ch <- []*ServiceInstance{}
	}

	// 监听上下文关闭事件
	go func() {
		<-ctx.Done()
		r.mu.Lock()
		defer r.mu.Unlock()

		if watches, ok := r.watches[serviceName]; ok {
			// 移除此通道
			for i, watch := range watches {
				if watch == ch {
					r.watches[serviceName] = append(watches[:i], watches[i+1:]...)
					break
				}
			}

			// 如果没有监听者，删除整个服务监听
			if len(r.watches[serviceName]) == 0 {
				delete(r.watches, serviceName)
			}
		}

		close(ch)
	}()

	return ch, nil
}
