package protocol

import (
	"context"
	"testing"
	"time"
)

// MockServiceDiscovery 用于测试的服务发现实现
type MockServiceDiscovery struct {
	instances map[string][]ServiceInstance
}

func NewMockServiceDiscovery() *MockServiceDiscovery {
	return &MockServiceDiscovery{
		instances: make(map[string][]ServiceInstance),
	}
}

func (sd *MockServiceDiscovery) GetServiceInstances(serviceName string) ([]ServiceInstance, error) {
	instances, ok := sd.instances[serviceName]
	if !ok {
		return []ServiceInstance{}, nil
	}
	return instances, nil
}

func (sd *MockServiceDiscovery) RegisterServiceInstance(instance ServiceInstance) error {
	if _, ok := sd.instances[instance.ServiceName]; !ok {
		sd.instances[instance.ServiceName] = []ServiceInstance{}
	}
	sd.instances[instance.ServiceName] = append(sd.instances[instance.ServiceName], instance)
	return nil
}

func (sd *MockServiceDiscovery) DeregisterServiceInstance(instanceID string) error {
	for serviceName, instances := range sd.instances {
		for i, instance := range instances {
			if instance.ID == instanceID {
				sd.instances[serviceName] = append(instances[:i], instances[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

func (sd *MockServiceDiscovery) WatchService(serviceName string, callback func([]ServiceInstance)) {
	// 简化版，不实现
}

func TestRoundRobinLoadBalancer(t *testing.T) {
	lb := NewRoundRobinLoadBalancer()

	// 创建测试实例
	instances := []ServiceInstance{
		{
			ID:          "instance-1",
			ServiceName: "test-service",
			Address:     "127.0.0.1:8001",
			Status:      "up",
		},
		{
			ID:          "instance-2",
			ServiceName: "test-service",
			Address:     "127.0.0.1:8002",
			Status:      "up",
		},
		{
			ID:          "instance-3",
			ServiceName: "test-service",
			Address:     "127.0.0.1:8003",
			Status:      "up",
		},
	}

	// 测试轮询
	selected1, err := lb.SelectInstance(instances)
	if err != nil {
		t.Fatalf("Failed to select instance: %v", err)
	}
	if selected1.ID != "instance-1" {
		t.Errorf("Expected instance-1, got %s", selected1.ID)
	}

	selected2, err := lb.SelectInstance(instances)
	if err != nil {
		t.Fatalf("Failed to select instance: %v", err)
	}
	if selected2.ID != "instance-2" {
		t.Errorf("Expected instance-2, got %s", selected2.ID)
	}

	selected3, err := lb.SelectInstance(instances)
	if err != nil {
		t.Fatalf("Failed to select instance: %v", err)
	}
	if selected3.ID != "instance-3" {
		t.Errorf("Expected instance-3, got %s", selected3.ID)
	}

	// 第四次调用应该回到第一个实例
	selected4, err := lb.SelectInstance(instances)
	if err != nil {
		t.Fatalf("Failed to select instance: %v", err)
	}
	if selected4.ID != "instance-1" {
		t.Errorf("Expected instance-1, got %s", selected4.ID)
	}
}

func TestWeightedLoadBalancer(t *testing.T) {
	lb := NewWeightedLoadBalancer()

	// 创建带权重的测试实例
	instances := []ServiceInstance{
		{
			ID:          "instance-1",
			ServiceName: "test-service",
			Address:     "127.0.0.1:8001",
			Weight:      1,
			Status:      "up",
		},
		{
			ID:          "instance-2",
			ServiceName: "test-service",
			Address:     "127.0.0.1:8002",
			Weight:      2,
			Status:      "up",
		},
		{
			ID:          "instance-3",
			ServiceName: "test-service",
			Address:     "127.0.0.1:8003",
			Weight:      3,
			Status:      "up",
		},
	}

	// 统计每个实例被选中的次数
	counts := make(map[string]int)

	// 运行多次选择
	totalRuns := 600
	for i := 0; i < totalRuns; i++ {
		instance, err := lb.SelectInstance(instances)
		if err != nil {
			t.Fatalf("Failed to select instance: %v", err)
		}
		counts[instance.ID]++
	}

	// 验证权重效果
	totalWeight := 6 // 1+2+3
	expectedRatio1 := float64(1) / float64(totalWeight)
	expectedRatio2 := float64(2) / float64(totalWeight)
	expectedRatio3 := float64(3) / float64(totalWeight)

	ratio1 := float64(counts["instance-1"]) / float64(totalRuns)
	ratio2 := float64(counts["instance-2"]) / float64(totalRuns)
	ratio3 := float64(counts["instance-3"]) / float64(totalRuns)

	// 允许5%的误差
	errorMargin := 0.05

	if ratio1 < expectedRatio1-errorMargin || ratio1 > expectedRatio1+errorMargin {
		t.Errorf("instance-1 ratio %f not within %f of expected %f", ratio1, errorMargin, expectedRatio1)
	}

	if ratio2 < expectedRatio2-errorMargin || ratio2 > expectedRatio2+errorMargin {
		t.Errorf("instance-2 ratio %f not within %f of expected %f", ratio2, errorMargin, expectedRatio2)
	}

	if ratio3 < expectedRatio3-errorMargin || ratio3 > expectedRatio3+errorMargin {
		t.Errorf("instance-3 ratio %f not within %f of expected %f", ratio3, errorMargin, expectedRatio3)
	}
}

func TestMessageRouter(t *testing.T) {
	// 创建测试组件
	sd := NewMockServiceDiscovery()
	lb := NewRoundRobinLoadBalancer()
	converter := NewProtocolConverter()
	registry := NewHandlerRegistry()

	// 创建消息路由器
	router := NewMessageRouter(sd, lb, converter, registry)

	// 注册服务类型和名称映射
	router.RegisterServiceType(ServiceTypeAuth, "auth-service")
	router.RegisterServiceType(ServiceTypeChat, "chat-service")

	// 注册服务实例
	sd.RegisterServiceInstance(ServiceInstance{
		ID:          "auth-1",
		ServiceName: "auth-service",
		Address:     "127.0.0.1:9001",
		Status:      "up",
	})

	sd.RegisterServiceInstance(ServiceInstance{
		ID:          "chat-1",
		ServiceName: "chat-service",
		Address:     "127.0.0.1:9002",
		Status:      "up",
	})

	// 测试获取服务名
	serviceName, ok := router.GetServiceName(ServiceTypeAuth)
	if !ok {
		t.Fatalf("Failed to get service name for Auth service")
	}
	if serviceName != "auth-service" {
		t.Errorf("Expected service name 'auth-service', got '%s'", serviceName)
	}

	// 测试路由消息
	msg := NewMessage(ServiceTypeAuth, 1, []byte("test payload"))

	// 设置较短的超时时间，避免测试时间过长
	router.SetTimeout(300 * time.Millisecond)

	ctx := context.Background()
	response, err := router.Route(ctx, msg)
	if err != nil {
		t.Fatalf("Failed to route message: %v", err)
	}

	if response == nil {
		t.Fatalf("Expected non-nil response")
	}

	if response.ServiceType != ServiceTypeAuth {
		t.Errorf("Expected service type %v, got %v", ServiceTypeAuth, response.ServiceType)
	}

	// 测试未注册的服务类型
	badMsg := NewMessage(ServiceTypeGame, 1, []byte("test payload"))
	_, err = router.Route(ctx, badMsg)
	if err == nil {
		t.Errorf("Expected error for unregistered service type")
	}
}

func TestInMemoryServiceDiscovery(t *testing.T) {
	sd := NewInMemoryServiceDiscovery()

	// 注册服务实例
	instance1 := ServiceInstance{
		ID:          "test-1",
		ServiceName: "test-service",
		Address:     "127.0.0.1:8001",
		Status:      "up",
	}

	err := sd.RegisterServiceInstance(instance1)
	if err != nil {
		t.Fatalf("Failed to register service instance: %v", err)
	}

	// 获取服务实例
	instances, err := sd.GetServiceInstances("test-service")
	if err != nil {
		t.Fatalf("Failed to get service instances: %v", err)
	}

	if len(instances) != 1 {
		t.Fatalf("Expected 1 instance, got %d", len(instances))
	}

	if instances[0].ID != "test-1" {
		t.Errorf("Expected instance ID 'test-1', got '%s'", instances[0].ID)
	}

	// 注册第二个实例
	instance2 := ServiceInstance{
		ID:          "test-2",
		ServiceName: "test-service",
		Address:     "127.0.0.1:8002",
		Status:      "up",
	}

	err = sd.RegisterServiceInstance(instance2)
	if err != nil {
		t.Fatalf("Failed to register second service instance: %v", err)
	}

	// 再次获取服务实例
	instances, err = sd.GetServiceInstances("test-service")
	if err != nil {
		t.Fatalf("Failed to get service instances: %v", err)
	}

	if len(instances) != 2 {
		t.Fatalf("Expected 2 instances, got %d", len(instances))
	}

	// 注销服务实例
	err = sd.DeregisterServiceInstance("test-1")
	if err != nil {
		t.Fatalf("Failed to deregister service instance: %v", err)
	}

	// 验证实例已被注销
	instances, err = sd.GetServiceInstances("test-service")
	if err != nil {
		t.Fatalf("Failed to get service instances: %v", err)
	}

	if len(instances) != 1 {
		t.Fatalf("Expected 1 instance after deregistration, got %d", len(instances))
	}

	if instances[0].ID != "test-2" {
		t.Errorf("Expected instance ID 'test-2', got '%s'", instances[0].ID)
	}
}
