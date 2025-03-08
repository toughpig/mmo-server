package rpc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInMemoryRegistry(t *testing.T) {
	// 创建注册表
	registry := NewInMemoryRegistry()
	ctx := context.Background()

	// 测试: 注册一个新实例
	instance1 := &ServiceInstance{
		ID:       "test-1",
		Name:     "test-service",
		Endpoint: "localhost:8001",
		Status:   "active",
		Weight:   1,
	}

	err := registry.Register(ctx, instance1)
	assert.NoError(t, err)

	// 测试: 获取服务实例
	instances, err := registry.GetInstances(ctx, "test-service")
	assert.NoError(t, err)
	assert.Len(t, instances, 1)
	assert.Equal(t, "test-1", instances[0].ID)

	// 测试: 注册另一个实例
	instance2 := &ServiceInstance{
		ID:       "test-2",
		Name:     "test-service",
		Endpoint: "localhost:8002",
		Status:   "active",
		Weight:   2,
	}

	err = registry.Register(ctx, instance2)
	assert.NoError(t, err)

	// 再次获取服务实例，应该有两个
	instances, err = registry.GetInstances(ctx, "test-service")
	assert.NoError(t, err)
	assert.Len(t, instances, 2)

	// 测试: 注销一个实例
	err = registry.Deregister(ctx, "test-1")
	assert.NoError(t, err)

	// 获取服务实例，应该只剩一个（实例会被标记为down，但不会立即删除）
	instances, err = registry.GetInstances(ctx, "test-service")
	assert.NoError(t, err)
	assert.Len(t, instances, 1)
	assert.Equal(t, "test-2", instances[0].ID)

	// 测试: 获取不存在的服务
	instances, err = registry.GetInstances(ctx, "non-existent-service")
	assert.NoError(t, err)
	assert.Len(t, instances, 0)
}

func TestWatchService(t *testing.T) {
	registry := NewInMemoryRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建一个通道监听服务变化
	watch, err := registry.WatchService(ctx, "watch-service")
	assert.NoError(t, err)

	// 启动一个goroutine从watch接收更新
	var instancesReceived [][]*ServiceInstance
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case instances, ok := <-watch:
				if !ok {
					return
				}
				instancesReceived = append(instancesReceived, instances)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 等待一段时间以确保goroutine已经开始
	time.Sleep(100 * time.Millisecond)

	// 注册一个实例，应该触发监听器
	instance1 := &ServiceInstance{
		ID:       "watch-1",
		Name:     "watch-service",
		Endpoint: "localhost:9001",
		Status:   "active",
	}
	registry.Register(ctx, instance1)

	// 再等待以确保监听器接收到更新
	time.Sleep(100 * time.Millisecond)

	// 注册第二个实例
	instance2 := &ServiceInstance{
		ID:       "watch-2",
		Name:     "watch-service",
		Endpoint: "localhost:9002",
		Status:   "active",
	}
	registry.Register(ctx, instance2)

	// 再等待以确保监听器接收到更新
	time.Sleep(100 * time.Millisecond)

	// 注销一个实例
	registry.Deregister(ctx, "watch-1")

	// 再等待以确保监听器接收到更新
	time.Sleep(100 * time.Millisecond)

	// 终止监听
	cancel()
	wg.Wait()

	// 验证：应该收到至少3条更新
	// 1. 初始空列表
	// 2. 添加第一个实例后的列表
	// 3. 添加第二个实例后的列表
	// 4. 注销第一个实例后的列表（可能被合并了）
	assert.GreaterOrEqual(t, len(instancesReceived), 3)

	// 验证最终状态：应该只有一个活跃实例
	instances, _ := registry.GetInstances(ctx, "watch-service")
	assert.Len(t, instances, 1)
	assert.Equal(t, "watch-2", instances[0].ID)
}

func TestLoadBalancer(t *testing.T) {
	registry := NewInMemoryRegistry()
	lb := NewLoadBalancer(registry)
	ctx := context.Background()

	// 注册三个测试服务实例
	for i := 1; i <= 3; i++ {
		instance := &ServiceInstance{
			ID:       fmt.Sprintf("lb-test-%d", i),
			Name:     "lb-service",
			Endpoint: fmt.Sprintf("localhost:700%d", i),
			Status:   "active",
			Weight:   i, // 权重分别为1,2,3
		}
		registry.Register(ctx, instance)
	}

	// 测试: 默认随机选择器
	instance, err := lb.GetInstance(ctx, "lb-service")
	assert.NoError(t, err)
	assert.Contains(t, instance.ID, "lb-test-")

	// 测试: 轮询选择器
	lb.SetSelector("lb-service", &RoundRobinSelector{})

	// 轮询应该按顺序选择实例
	instance1, _ := lb.GetInstance(ctx, "lb-service")
	instance2, _ := lb.GetInstance(ctx, "lb-service")
	instance3, _ := lb.GetInstance(ctx, "lb-service")
	instance4, _ := lb.GetInstance(ctx, "lb-service")

	// 第四次应该回到第一个实例
	assert.Equal(t, instance1.ID, instance4.ID)

	// 测试：实例应该是轮流的（不一定是连续的1,2,3，但每个实例应该都不同）
	assert.NotEqual(t, instance1.ID, instance2.ID)
	assert.NotEqual(t, instance2.ID, instance3.ID)

	// 测试: 加权选择器
	lb.SetSelector("lb-service", &WeightedSelector{})

	// 模拟1000次选择，验证权重分布
	counts := make(map[string]int)
	totalTests := 1000

	for i := 0; i < totalTests; i++ {
		instance, _ := lb.GetInstance(ctx, "lb-service")
		counts[instance.ID]++
	}

	// 检查是否所有实例都被选到了
	assert.Len(t, counts, 3)

	// 检查实例3被选到的次数应该大约是实例1的3倍（因为权重是3:1）
	// 但因为随机性，我们用一个宽松的比例检查
	ratio := float64(counts["lb-test-3"]) / float64(counts["lb-test-1"])
	t.Logf("权重选择结果: %v, 权重比例: %.2f (期望接近3.0)", counts, ratio)
	assert.Greater(t, ratio, 1.5) // 权重比至少应该大于1.5

	// 测试：不存在的服务
	_, err = lb.GetInstance(ctx, "non-existent-service")
	assert.Error(t, err)
}

// 自定义选择器实现
type FirstSelector struct{}

func (s *FirstSelector) Select(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances available")
	}
	return instances[0], nil
}

func TestCustomSelector(t *testing.T) {
	registry := NewInMemoryRegistry()
	lb := NewLoadBalancer(registry)
	ctx := context.Background()

	// 注册测试实例
	for i := 1; i <= 3; i++ {
		instance := &ServiceInstance{
			ID:       fmt.Sprintf("custom-%d", i),
			Name:     "custom-service",
			Endpoint: fmt.Sprintf("localhost:800%d", i),
			Status:   "active",
		}
		registry.Register(ctx, instance)
	}

	// 设置自定义选择器
	lb.SetSelector("custom-service", &FirstSelector{})

	// 测试自定义选择器
	for i := 0; i < 5; i++ {
		instance, err := lb.GetInstance(ctx, "custom-service")
		assert.NoError(t, err)
		assert.Equal(t, "custom-1", instance.ID, "自定义选择器应该总是选择第一个实例")
	}
}
