package protocol

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	ErrServiceNotFound     = errors.New("service not found")
	ErrNoAvailableInstance = errors.New("no available service instance")
	ErrRoutingFailed       = errors.New("message routing failed")
	ErrTimeout             = errors.New("routing timeout")
)

// ServiceInstance 服务实例
type ServiceInstance struct {
	ID          string
	ServiceName string
	Address     string
	Weight      int       // 权重，用于负载均衡
	Status      string    // 服务状态："up", "down", "maintenance"
	LastSeen    time.Time // 最后一次心跳时间
	Metadata    map[string]string
}

// ServiceDiscovery 服务发现接口
type ServiceDiscovery interface {
	// GetServiceInstances 获取服务实例列表
	GetServiceInstances(serviceName string) ([]ServiceInstance, error)

	// RegisterServiceInstance 注册服务实例
	RegisterServiceInstance(instance ServiceInstance) error

	// DeregisterServiceInstance 注销服务实例
	DeregisterServiceInstance(instanceID string) error

	// WatchService 监视服务变化
	WatchService(serviceName string, callback func([]ServiceInstance))
}

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	// SelectInstance 从实例列表中选择一个
	SelectInstance(instances []ServiceInstance) (*ServiceInstance, error)
}

// RoundRobinLoadBalancer 轮询负载均衡器
type RoundRobinLoadBalancer struct {
	mu      sync.Mutex
	counter map[string]int
}

// NewRoundRobinLoadBalancer 创建轮询负载均衡器
func NewRoundRobinLoadBalancer() *RoundRobinLoadBalancer {
	return &RoundRobinLoadBalancer{
		counter: make(map[string]int),
	}
}

// SelectInstance 从实例列表中选择一个（轮询方式）
func (lb *RoundRobinLoadBalancer) SelectInstance(instances []ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoAvailableInstance
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	serviceName := instances[0].ServiceName
	count := lb.counter[serviceName]

	// 筛选可用实例
	var availableInstances []ServiceInstance
	for _, instance := range instances {
		if instance.Status == "up" {
			availableInstances = append(availableInstances, instance)
		}
	}

	if len(availableInstances) == 0 {
		return nil, ErrNoAvailableInstance
	}

	// 选择实例
	selectedIndex := count % len(availableInstances)
	lb.counter[serviceName] = count + 1

	return &availableInstances[selectedIndex], nil
}

// WeightedLoadBalancer 带权重的负载均衡器
type WeightedLoadBalancer struct {
	mu      sync.Mutex
	counter map[string]int
}

// NewWeightedLoadBalancer 创建带权重的负载均衡器
func NewWeightedLoadBalancer() *WeightedLoadBalancer {
	return &WeightedLoadBalancer{
		counter: make(map[string]int),
	}
}

// SelectInstance 按权重选择实例
func (lb *WeightedLoadBalancer) SelectInstance(instances []ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoAvailableInstance
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	serviceName := instances[0].ServiceName

	// 筛选可用实例
	var availableInstances []ServiceInstance
	totalWeight := 0
	for _, instance := range instances {
		if instance.Status == "up" {
			availableInstances = append(availableInstances, instance)
			totalWeight += instance.Weight
		}
	}

	if len(availableInstances) == 0 {
		return nil, ErrNoAvailableInstance
	}

	// 如果没有设置权重，使用轮询
	if totalWeight == 0 {
		count := lb.counter[serviceName]
		selectedIndex := count % len(availableInstances)
		lb.counter[serviceName] = count + 1
		return &availableInstances[selectedIndex], nil
	}

	// 使用权重选择
	count := lb.counter[serviceName] % totalWeight
	lb.counter[serviceName]++

	runningTotal := 0
	for i, instance := range availableInstances {
		runningTotal += instance.Weight
		if count < runningTotal {
			return &availableInstances[i], nil
		}
	}

	// 兜底，使用第一个
	return &availableInstances[0], nil
}

// MessageRouter 消息路由器
type MessageRouter struct {
	serviceDiscovery ServiceDiscovery
	loadBalancer     LoadBalancer
	converter        ProtocolConverter
	registry         *HandlerRegistry
	serviceMap       map[ServiceType]string // 服务类型 -> 服务名
	timeout          time.Duration
	mu               sync.RWMutex
}

// NewMessageRouter 创建消息路由器
func NewMessageRouter(sd ServiceDiscovery, lb LoadBalancer, converter ProtocolConverter, registry *HandlerRegistry) *MessageRouter {
	return &MessageRouter{
		serviceDiscovery: sd,
		loadBalancer:     lb,
		converter:        converter,
		registry:         registry,
		serviceMap:       make(map[ServiceType]string),
		timeout:          30 * time.Second, // 默认30秒超时
	}
}

// RegisterServiceType 注册服务类型和服务名映射
func (r *MessageRouter) RegisterServiceType(serviceType ServiceType, serviceName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.serviceMap[serviceType] = serviceName
}

// GetServiceName 获取服务类型对应的服务名
func (r *MessageRouter) GetServiceName(serviceType ServiceType) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.serviceMap[serviceType]
	return name, ok
}

// SetTimeout 设置路由超时时间
func (r *MessageRouter) SetTimeout(timeout time.Duration) {
	r.timeout = timeout
}

// Route 路由消息
func (r *MessageRouter) Route(ctx context.Context, msg *Message) (*Message, error) {
	// 检查目标服务类型
	serviceName, ok := r.GetServiceName(msg.ServiceType)
	if !ok {
		return nil, fmt.Errorf("%w: service type %d", ErrServiceNotFound, msg.ServiceType)
	}

	// 设置服务名
	if msg.DestinationService == "" {
		msg.DestinationService = serviceName
	}

	// 查找服务实例
	instances, err := r.serviceDiscovery.GetServiceInstances(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get service instances: %w", err)
	}

	// 负载均衡选择实例
	instance, err := r.loadBalancer.SelectInstance(instances)
	if err != nil {
		return nil, fmt.Errorf("load balancing failed: %w", err)
	}

	// 创建上下文超时
	timeoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// 发送消息到目标服务（这里是模拟实现）
	responseCh := make(chan *Message, 1)
	errCh := make(chan error, 1)

	go func() {
		log.Printf("Routing message to service %s, instance %s at %s",
			serviceName, instance.ID, instance.Address)

		// 在实际实现中，这里会将消息发送到目标服务，并等待响应
		// 为了演示，我们模拟一个应答
		time.Sleep(100 * time.Millisecond)

		// 创建一个简单的响应消息
		response := &Message{
			Version:            msg.Version,
			Flags:              msg.Flags,
			ServiceType:        msg.ServiceType,
			MessageType:        msg.MessageType,
			SequenceID:         msg.SequenceID,
			Timestamp:          uint64(time.Now().UnixMicro()),
			PayloadLength:      0,
			SourceService:      serviceName,
			DestinationService: msg.SourceService,
			CorrelationID:      msg.CorrelationID,
			SessionID:          msg.SessionID,
			Payload:            []byte("Response placeholder"),
		}

		responseCh <- response
	}()

	// 等待响应或超时
	select {
	case response := <-responseCh:
		return response, nil
	case err := <-errCh:
		return nil, err
	case <-timeoutCtx.Done():
		return nil, ErrTimeout
	}
}

// RouteAsync 异步路由消息，不等待响应
func (r *MessageRouter) RouteAsync(ctx context.Context, msg *Message) error {
	// 检查目标服务类型
	serviceName, ok := r.GetServiceName(msg.ServiceType)
	if !ok {
		return fmt.Errorf("%w: service type %d", ErrServiceNotFound, msg.ServiceType)
	}

	// 设置服务名
	if msg.DestinationService == "" {
		msg.DestinationService = serviceName
	}

	// 查找服务实例
	instances, err := r.serviceDiscovery.GetServiceInstances(serviceName)
	if err != nil {
		return fmt.Errorf("failed to get service instances: %w", err)
	}

	// 负载均衡选择实例
	instance, err := r.loadBalancer.SelectInstance(instances)
	if err != nil {
		return fmt.Errorf("load balancing failed: %w", err)
	}

	// 异步发送消息
	go func() {
		log.Printf("Async routing message to service %s, instance %s at %s",
			serviceName, instance.ID, instance.Address)

		// 在实际实现中，这里会将消息发送到目标服务
		// 为了演示，我们只记录日志
		time.Sleep(50 * time.Millisecond)
		log.Printf("Message sent to %s", instance.Address)
	}()

	return nil
}

// Broadcast 广播消息到某服务的所有实例
func (r *MessageRouter) Broadcast(ctx context.Context, msg *Message, serviceType ServiceType) error {
	// 检查目标服务类型
	serviceName, ok := r.GetServiceName(serviceType)
	if !ok {
		return fmt.Errorf("%w: service type %d", ErrServiceNotFound, serviceType)
	}

	// 设置服务名
	msg.DestinationService = serviceName
	msg.ServiceType = serviceType

	// 查找服务实例
	instances, err := r.serviceDiscovery.GetServiceInstances(serviceName)
	if err != nil {
		return fmt.Errorf("failed to get service instances: %w", err)
	}

	if len(instances) == 0 {
		return ErrNoAvailableInstance
	}

	// 向所有实例广播消息
	for _, instance := range instances {
		if instance.Status != "up" {
			continue
		}

		// 异步广播
		go func(inst ServiceInstance) {
			log.Printf("Broadcasting message to service %s, instance %s at %s",
				serviceName, inst.ID, inst.Address)

			// 在实际实现中，这里会将消息发送到目标服务
			// 为了演示，我们只记录日志
			time.Sleep(50 * time.Millisecond)
			log.Printf("Broadcast message sent to %s", inst.Address)
		}(instance)
	}

	return nil
}

// 内存中的简单服务发现实现（仅用于测试）
type InMemoryServiceDiscovery struct {
	instances map[string][]ServiceInstance
	watches   map[string][]func([]ServiceInstance)
	mu        sync.RWMutex
}

// NewInMemoryServiceDiscovery 创建内存服务发现
func NewInMemoryServiceDiscovery() *InMemoryServiceDiscovery {
	return &InMemoryServiceDiscovery{
		instances: make(map[string][]ServiceInstance),
		watches:   make(map[string][]func([]ServiceInstance)),
	}
}

// GetServiceInstances 获取服务实例列表
func (sd *InMemoryServiceDiscovery) GetServiceInstances(serviceName string) ([]ServiceInstance, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	instances, ok := sd.instances[serviceName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, serviceName)
	}

	return instances, nil
}

// RegisterServiceInstance 注册服务实例
func (sd *InMemoryServiceDiscovery) RegisterServiceInstance(instance ServiceInstance) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	instances := sd.instances[instance.ServiceName]

	// 检查是否已存在
	for i, inst := range instances {
		if inst.ID == instance.ID {
			// 更新已存在的实例
			instances[i] = instance
			sd.instances[instance.ServiceName] = instances
			sd.notifyWatchers(instance.ServiceName)
			return nil
		}
	}

	// 添加新实例
	sd.instances[instance.ServiceName] = append(instances, instance)
	sd.notifyWatchers(instance.ServiceName)

	return nil
}

// DeregisterServiceInstance 注销服务实例
func (sd *InMemoryServiceDiscovery) DeregisterServiceInstance(instanceID string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	for serviceName, instances := range sd.instances {
		for i, instance := range instances {
			if instance.ID == instanceID {
				// 移除实例
				sd.instances[serviceName] = append(instances[:i], instances[i+1:]...)
				sd.notifyWatchers(serviceName)
				return nil
			}
		}
	}

	return fmt.Errorf("instance not found: %s", instanceID)
}

// WatchService 监视服务变化
func (sd *InMemoryServiceDiscovery) WatchService(serviceName string, callback func([]ServiceInstance)) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.watches[serviceName] = append(sd.watches[serviceName], callback)
}

// notifyWatchers 通知监视器
func (sd *InMemoryServiceDiscovery) notifyWatchers(serviceName string) {
	watches := sd.watches[serviceName]
	instances := sd.instances[serviceName]

	for _, watch := range watches {
		go watch(instances)
	}
}
