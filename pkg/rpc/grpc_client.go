package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "mmo-server/proto_define"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// GRPCClient implements the RPCClient interface using gRPC
type GRPCClient struct {
	endpoint    string
	rpcClient   pb.RPCServiceClient
	seq         uint64
	pending     map[uint64]*Call
	pendingLock sync.Mutex
	closing     bool
	closed      bool
	mutex       sync.Mutex
}

// NewGRPCClient creates a new client with gRPC transport
func NewGRPCClient(endpoint string) (*GRPCClient, error) {
	// Get the RPC client from connection manager
	rpcClient, err := GetRPCServiceClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get RPC client for %s: %w", endpoint, err)
	}

	grpcClient := &GRPCClient{
		endpoint:  endpoint,
		rpcClient: rpcClient,
		pending:   make(map[uint64]*Call),
	}

	return grpcClient, nil
}

// nextSequence generates the next sequence number
func (c *GRPCClient) nextSequence() uint64 {
	return atomic.AddUint64(&c.seq, 1)
}

// Call performs a synchronous RPC call
func (c *GRPCClient) Call(ctx context.Context, serviceMethod string, args proto.Message, reply proto.Message) error {
	// Check if client is closed
	c.mutex.Lock()
	if c.closing || c.closed {
		c.mutex.Unlock()
		return ErrConnectionClosed
	}
	c.mutex.Unlock()

	// Pack the args into Any
	argsAny, err := anypb.New(args)
	if err != nil {
		return fmt.Errorf("failed to pack args: %w", err)
	}

	// Create the RPC request
	sequence := c.nextSequence()
	req := &pb.RPCRequest{
		ServiceMethod: serviceMethod,
		Sequence:      sequence,
		Args:          argsAny,
	}

	// 实现重试逻辑
	maxRetries := 3
	retryBackoff := 100 * time.Millisecond
	var lastErr error

	for retry := 0; retry < maxRetries; retry++ {
		// 如果不是第一次尝试，则等待一段时间
		if retry > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(retryBackoff):
				// 指数退避增加等待时间
				retryBackoff *= 2
			}

			log.Printf("Retrying RPC call %s (attempt %d/%d)", serviceMethod, retry+1, maxRetries)
		}

		// 检查客户端是否已关闭
		c.mutex.Lock()
		if c.closing || c.closed {
			c.mutex.Unlock()
			return ErrConnectionClosed
		}
		c.mutex.Unlock()

		// Make the gRPC call
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := c.rpcClient.Call(callCtx, req)
		cancel()

		if err == nil {
			// 检查响应中的错误
			if resp.Error != "" {
				lastErr = errors.New(resp.Error)

				// 判断是否应该重试（例如，不重试非临时性错误）
				if !isRetryableError(resp.Error) {
					return lastErr
				}

				continue // 重试
			}

			// 成功响应，解包并返回
			if resp.Reply != nil {
				err = resp.Reply.UnmarshalTo(reply)
				if err != nil {
					return fmt.Errorf("failed to unpack reply: %w", err)
				}
			}

			return nil
		}

		lastErr = fmt.Errorf("gRPC call failed: %w", err)

		// 判断错误是否可重试
		if !isRetryableGRPCError(err) {
			break
		}
	}

	// 所有重试都失败了
	return fmt.Errorf("all retries failed for %s: %w", serviceMethod, lastErr)
}

// 判断错误是否可重试
func isRetryableError(errMsg string) bool {
	// 根据错误消息判断是否可重试
	// 这里只是示例，根据实际情况判断
	if strings.Contains(errMsg, "not available") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "temporarily unavailable") {
		return true
	}
	return false
}

// 判断gRPC错误是否可重试
func isRetryableGRPCError(err error) bool {
	// 根据gRPC错误码判断是否可重试
	code := status.Code(err)
	return code == codes.Unavailable ||
		code == codes.DeadlineExceeded ||
		code == codes.ResourceExhausted ||
		code == codes.Aborted
}

// Go starts an asynchronous RPC call
func (c *GRPCClient) Go(ctx context.Context, serviceMethod string, args proto.Message, reply proto.Message, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10) // buffered
	}

	// Check if client is closed
	c.mutex.Lock()
	if c.closing || c.closed {
		c.mutex.Unlock()
		call := &Call{
			ServiceMethod: serviceMethod,
			Args:          args,
			Reply:         reply,
			Error:         ErrConnectionClosed,
			Done:          done,
		}
		call.done()
		return call
	}
	c.mutex.Unlock()

	// Create a context with a timeout if the parent context doesn't have one
	callCtx := ctx
	var cancelFunc context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		// Default timeout of 60 seconds
		callCtx, cancelFunc = context.WithTimeout(ctx, 60*time.Second)
	}

	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
		ctx:           callCtx,
		cancelFunc:    cancelFunc,
	}

	// Pack the args into Any
	argsAny, err := anypb.New(args)
	if err != nil {
		call.Error = fmt.Errorf("failed to pack args: %w", err)
		call.done()
		return call
	}

	// Create the RPC request
	sequence := c.nextSequence()
	req := &pb.RPCRequest{
		ServiceMethod: serviceMethod,
		Sequence:      sequence,
		Args:          argsAny,
	}

	// Register call in pending map
	c.pendingLock.Lock()
	c.pending[sequence] = call
	c.pendingLock.Unlock()

	// Make the gRPC call in a goroutine
	go func() {
		resp, err := c.rpcClient.Call(callCtx, req)

		// Remove the call from pending
		c.pendingLock.Lock()
		delete(c.pending, sequence)
		c.pendingLock.Unlock()

		if err != nil {
			call.Error = fmt.Errorf("gRPC call failed: %w", err)
			call.done()
			return
		}

		// Check for error in response
		if resp.Error != "" {
			call.Error = errors.New(resp.Error)
			call.done()
			return
		}

		// Unpack the reply
		if resp.Reply != nil {
			err = resp.Reply.UnmarshalTo(call.Reply)
			if err != nil {
				call.Error = fmt.Errorf("failed to unpack reply: %w", err)
				call.done()
				return
			}
		}

		call.done()
	}()

	return call
}

// Close closes the client connection
func (c *GRPCClient) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil
	}

	c.closing = true

	// Cancel all pending calls
	c.pendingLock.Lock()
	for _, call := range c.pending {
		call.Error = ErrConnectionClosed
		call.done()
	}
	c.pending = make(map[uint64]*Call)
	c.pendingLock.Unlock()

	// We don't close the connection here as it's managed by ConnManager
	c.closed = true
	c.closing = false

	return nil
}
