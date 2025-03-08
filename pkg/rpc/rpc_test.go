package rpc

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"testing"
	"time"

	pb "mmo-server/proto_define"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestService 是一个用于测试的RPC服务
type TestService struct {
	callCount int
	mu        sync.Mutex
}

// EchoRequest 实现一个简单的回显请求
func (s *TestService) EchoRequest(ctx context.Context, req *pb.ChatRequest, resp *pb.ChatResponse) error {
	s.mu.Lock()
	s.callCount++
	s.mu.Unlock()

	resp.MessageId = int64(100 + s.callCount)
	resp.Delivered = true
	resp.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
	resp.Header = &pb.MessageHeader{
		MsgId:      req.Header.MsgId,
		Timestamp:  time.Now().UnixNano() / int64(time.Millisecond),
		SessionId:  req.Header.SessionId,
		Version:    req.Header.Version,
		ResultCode: 0,
		ResultMsg:  "Success",
	}

	return nil
}

// GetCallCount 返回服务被调用的次数
func (s *TestService) GetCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callCount
}

// SlowService 是一个延迟响应的服务，用于测试超时
type SlowService struct{}

// SlowMethod 是一个会休眠500毫秒的方法
func (s *SlowService) SlowMethod(ctx context.Context, req *pb.ChatRequest, resp *pb.ChatResponse) error {
	time.Sleep(500 * time.Millisecond)
	resp.Delivered = true
	return nil
}

// ErrorService 是一个总是返回错误的服务，用于测试错误处理
type ErrorService struct{}

// ErrorMethod 是一个总是返回错误的方法
func (s *ErrorService) ErrorMethod(ctx context.Context, req *pb.ChatRequest, resp *pb.ChatResponse) error {
	return fmt.Errorf("模拟的服务错误")
}

// TestRPCBasicFunctionality 测试RPC的基本功能
func TestRPCBasicFunctionality(t *testing.T) {
	// Unix socket不再适用于gRPC，切换为TCP
	tcpAddress := fmt.Sprintf("localhost:%d", 50100+rand.Intn(1000))

	// 创建服务器
	server := NewRPCServer(tcpAddress, TransportGRPC)

	// 注册测试服务
	testService := &TestService{}
	err := server.Register(testService)
	require.NoError(t, err, "无法注册测试服务")

	// 启动服务器
	err = server.Start()
	require.NoError(t, err, "无法启动RPC服务器")
	defer server.Stop()

	// 给服务器一点时间启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client, err := NewRPCClient(tcpAddress, TransportGRPC)
	require.NoError(t, err, "无法创建RPC客户端")
	defer client.Close()

	// 创建请求
	req := &pb.ChatRequest{
		Header: &pb.MessageHeader{
			MsgId:     int32(pb.MessageType_CHAT_REQUEST),
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			SessionId: "test-session-123",
			Version:   1,
		},
		SenderId:   "user1",
		SenderName: "TestUser",
		Content:    "Hello, RPC!",
		ChatType:   pb.ChatType_WORLD,
	}

	// 创建响应对象
	resp := &pb.ChatResponse{}

	// 发送RPC请求
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = client.Call(ctx, "TestService.EchoRequest", req, resp)
	require.NoError(t, err, "RPC调用失败")

	// 验证响应
	assert.True(t, resp.Delivered, "消息未被标记为已送达")
	assert.Equal(t, int64(101), resp.MessageId, "消息ID不正确")
	assert.NotZero(t, resp.Timestamp, "时间戳应该非零")
	assert.Equal(t, req.Header.MsgId, resp.Header.MsgId, "消息ID应该匹配")
	assert.Equal(t, req.Header.SessionId, resp.Header.SessionId, "会话ID应该匹配")
	assert.Equal(t, int32(0), resp.Header.ResultCode, "结果代码应该为0")

	// 验证服务被调用了一次
	assert.Equal(t, 1, testService.GetCallCount(), "服务应该被调用一次")
}

// TestRPCMultipleCalls 测试多次RPC调用
func TestRPCMultipleCalls(t *testing.T) {
	// Unix socket不再适用于gRPC，切换为TCP
	tcpAddress := fmt.Sprintf("localhost:%d", 50100+rand.Intn(1000))

	// 创建服务器
	server := NewRPCServer(tcpAddress, TransportGRPC)

	// 注册测试服务
	testService := &TestService{}
	err := server.Register(testService)
	require.NoError(t, err, "无法注册测试服务")

	// 启动服务器
	err = server.Start()
	require.NoError(t, err, "无法启动RPC服务器")
	defer server.Stop()

	// 给服务器一点时间启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client, err := NewRPCClient(tcpAddress, TransportGRPC)
	require.NoError(t, err, "无法创建RPC客户端")
	defer client.Close()

	// 创建请求基础模板
	baseReq := &pb.ChatRequest{
		Header: &pb.MessageHeader{
			MsgId:     int32(pb.MessageType_CHAT_REQUEST),
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			SessionId: "test-session-123",
			Version:   1,
		},
		SenderId:   "user1",
		SenderName: "TestUser",
		ChatType:   pb.ChatType_WORLD,
	}

	// 进行多次调用
	numCalls := 5
	for i := 0; i < numCalls; i++ {
		req := *baseReq // 复制请求
		req.Content = fmt.Sprintf("Message %d", i+1)

		resp := &pb.ChatResponse{}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = client.Call(ctx, "TestService.EchoRequest", &req, resp)
		cancel()

		require.NoError(t, err, "第 %d 次RPC调用失败", i+1)
		assert.True(t, resp.Delivered, "第 %d 次，消息未被标记为已送达", i+1)
		assert.Equal(t, int64(101+i), resp.MessageId, "第 %d 次，消息ID不正确", i+1)
	}

	// 验证服务被调用了5次
	assert.Equal(t, numCalls, testService.GetCallCount(), "服务应该被调用 %d 次", numCalls)
}

// TestRPCTimeout 测试RPC超时
func TestRPCTimeout(t *testing.T) {
	// Unix socket不再适用于gRPC，切换为TCP
	tcpAddress := fmt.Sprintf("localhost:%d", 50100+rand.Intn(1000))

	// 创建服务器
	server := NewRPCServer(tcpAddress, TransportGRPC)

	// 注册一个会休眠的服务，以触发超时
	slowService := &SlowService{}
	err := server.Register(slowService)
	require.NoError(t, err, "无法注册慢速服务")

	// 启动服务器
	err = server.Start()
	require.NoError(t, err, "无法启动RPC服务器")
	defer server.Stop()

	// 给服务器一点时间启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client, err := NewRPCClient(tcpAddress, TransportGRPC)
	require.NoError(t, err, "无法创建RPC客户端")
	defer client.Close()

	// 创建请求
	req := &pb.ChatRequest{
		Header: &pb.MessageHeader{
			MsgId:     int32(pb.MessageType_CHAT_REQUEST),
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			SessionId: "test-session-123",
			Version:   1,
		},
		Content: "This call should timeout",
	}

	resp := &pb.ChatResponse{}

	// 使用很短的超时
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 应该超时
	err = client.Call(ctx, "SlowService.SlowMethod", req, resp)
	assert.Error(t, err, "调用应该超时")
}

// TestRPCConcurrentCalls 测试并发RPC调用
func TestRPCConcurrentCalls(t *testing.T) {
	// Unix socket不再适用于gRPC，切换为TCP
	tcpAddress := fmt.Sprintf("localhost:%d", 50100+rand.Intn(1000))

	// 创建服务器
	server := NewRPCServer(tcpAddress, TransportGRPC)

	// 注册测试服务
	testService := &TestService{}
	err := server.Register(testService)
	require.NoError(t, err, "无法注册测试服务")

	// 启动服务器
	err = server.Start()
	require.NoError(t, err, "无法启动RPC服务器")
	defer server.Stop()

	// 给服务器一点时间启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client, err := NewRPCClient(tcpAddress, TransportGRPC)
	require.NoError(t, err, "无法创建RPC客户端")
	defer client.Close()

	// 并发调用次数
	numCalls := 10
	var wg sync.WaitGroup
	wg.Add(numCalls)

	for i := 0; i < numCalls; i++ {
		go func(callID int) {
			defer wg.Done()

			req := &pb.ChatRequest{
				Header: &pb.MessageHeader{
					MsgId:     int32(pb.MessageType_CHAT_REQUEST),
					Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
					SessionId: fmt.Sprintf("test-session-%d", callID),
					Version:   1,
				},
				SenderId:   fmt.Sprintf("user%d", callID),
				SenderName: fmt.Sprintf("TestUser %d", callID),
				Content:    fmt.Sprintf("Concurrent message %d", callID),
				ChatType:   pb.ChatType_WORLD,
			}

			resp := &pb.ChatResponse{}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := client.Call(ctx, "TestService.EchoRequest", req, resp)
			if err != nil {
				t.Errorf("并发调用 %d 失败: %v", callID, err)
				return
			}

			if !resp.Delivered {
				t.Errorf("并发调用 %d: 消息未被标记为已送达", callID)
			}
		}(i)
	}

	// 等待所有调用完成
	wg.Wait()

	// 验证服务被调用了预期的次数
	assert.Equal(t, numCalls, testService.GetCallCount(), "服务应该被调用 %d 次", numCalls)
}

// TestRPCServiceError 测试服务错误处理
func TestRPCServiceError(t *testing.T) {
	// Unix socket不再适用于gRPC，切换为TCP
	tcpAddress := fmt.Sprintf("localhost:%d", 50100+rand.Intn(1000))

	// 创建服务器
	server := NewRPCServer(tcpAddress, TransportGRPC)

	// 注册一个会返回错误的服务
	errorService := &ErrorService{}
	err := server.Register(errorService)
	require.NoError(t, err, "无法注册错误服务")

	// 启动服务器
	err = server.Start()
	require.NoError(t, err, "无法启动RPC服务器")
	defer server.Stop()

	// 给服务器一点时间启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client, err := NewRPCClient(tcpAddress, TransportGRPC)
	require.NoError(t, err, "无法创建RPC客户端")
	defer client.Close()

	// 创建请求
	req := &pb.ChatRequest{
		Header: &pb.MessageHeader{
			MsgId:     int32(pb.MessageType_CHAT_REQUEST),
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			SessionId: "test-session-error",
			Version:   1,
		},
		Content: "This call should return an error",
	}

	resp := &pb.ChatResponse{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 应该返回错误
	err = client.Call(ctx, "ErrorService.ErrorMethod", req, resp)
	assert.Error(t, err, "调用应该返回错误")
	assert.Contains(t, err.Error(), "模拟的服务错误", "错误消息应该包含预期的错误")
}

// TestRPCInvalidService 测试调用不存在的服务
func TestRPCInvalidService(t *testing.T) {
	// Unix socket不再适用于gRPC，切换为TCP
	tcpAddress := fmt.Sprintf("localhost:%d", 50100+rand.Intn(1000))

	// 创建服务器
	server := NewRPCServer(tcpAddress, TransportGRPC)

	// 注册测试服务
	testService := &TestService{}
	err := server.Register(testService)
	require.NoError(t, err, "无法注册测试服务")

	// 启动服务器
	err = server.Start()
	require.NoError(t, err, "无法启动RPC服务器")
	defer server.Stop()

	// 给服务器一点时间启动
	time.Sleep(100 * time.Millisecond)

	// 创建客户端
	client, err := NewRPCClient(tcpAddress, TransportGRPC)
	require.NoError(t, err, "无法创建RPC客户端")
	defer client.Close()

	// 创建请求
	req := &pb.ChatRequest{
		Header: &pb.MessageHeader{
			MsgId:     int32(pb.MessageType_CHAT_REQUEST),
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			SessionId: "test-session-invalid",
			Version:   1,
		},
		Content: "This call should fail - invalid service",
	}

	resp := &pb.ChatResponse{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 尝试调用不存在的服务
	err = client.Call(ctx, "NonExistentService.SomeMethod", req, resp)
	assert.Error(t, err, "调用不存在的服务应该失败")
}

// 初始化测试
func init() {
	// 配置日志
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
