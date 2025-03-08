package db

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

// AsyncConfig 异步数据库访问配置
type AsyncConfig struct {
	// 工作队列大小
	QueueSize int
	// 工作线程数
	WorkerCount int
	// 操作超时时间
	OperationTimeout time.Duration
	// 重试次数
	MaxRetries int
	// 重试间隔
	RetryInterval time.Duration
	// 是否启用批处理
	EnableBatching bool
	// 批处理大小
	BatchSize int
	// 批处理间隔
	BatchInterval time.Duration
}

// DefaultAsyncConfig 返回默认异步配置
func DefaultAsyncConfig() *AsyncConfig {
	return &AsyncConfig{
		QueueSize:        1000,
		WorkerCount:      5,
		OperationTimeout: 30 * time.Second,
		MaxRetries:       3,
		RetryInterval:    500 * time.Millisecond,
		EnableBatching:   true,
		BatchSize:        100,
		BatchInterval:    1 * time.Second,
	}
}

// AsyncStats 异步操作统计信息
type AsyncStats struct {
	QueuedOperations    int64
	CompletedOperations int64
	FailedOperations    int64
	RetryCount          int64
	CurrentQueueSize    int
	AvgProcessingTimeMs float64
	BatchesProcessed    int64
	AvgBatchSizeItems   float64
}

// AsyncResult 异步操作结果
type AsyncResult struct {
	// 操作是否成功
	Success bool
	// 错误信息（如果有）
	Error error
	// 影响的行数（对于写操作）
	RowsAffected int64
	// 结果数据（对于读操作）
	Data interface{}
	// 操作完成时间
	CompletedAt time.Time
	// 操作耗时
	Duration time.Duration
}

// AsyncCallback 异步操作回调函数
type AsyncCallback func(result *AsyncResult)

// AsyncOperation 异步操作类型
type AsyncOperation struct {
	// 操作类型
	Type string
	// SQL语句
	SQL string
	// 参数
	Args []interface{}
	// 结果处理函数
	ResultHandler func(rows pgx.Rows) (interface{}, error)
	// 完成回调
	Callback AsyncCallback
	// 上下文
	Context context.Context
	// 提交时间
	SubmittedAt time.Time
	// 重试计数
	RetryCount int
	// 批处理ID（如果是批处理的一部分）
	BatchID string
}

// AsyncDB 异步数据库访问接口
type AsyncDB interface {
	// 异步执行查询
	QueryAsync(ctx context.Context, sql string, resultHandler func(rows pgx.Rows) (interface{}, error), callback AsyncCallback, args ...interface{}) error
	// 异步执行命令
	ExecAsync(ctx context.Context, sql string, callback AsyncCallback, args ...interface{}) error
	// 获取统计信息
	Stats() *AsyncStats
	// 等待所有操作完成
	WaitForCompletion(timeout time.Duration) error
	// 关闭异步数据库访问
	Close() error
}

// asyncDB 实现异步数据库访问
type asyncDB struct {
	pool        Pool
	config      *AsyncConfig
	queue       chan *AsyncOperation
	wg          sync.WaitGroup
	mu          sync.RWMutex
	stats       AsyncStats
	batchQueues map[string][]*AsyncOperation
	batchMu     sync.Mutex
	batchTimers map[string]*time.Timer
	stopChan    chan struct{}
	isClosed    bool
}

// NewAsyncDB 创建异步数据库访问
func NewAsyncDB(pool Pool, config *AsyncConfig) (AsyncDB, error) {
	if pool == nil {
		return nil, errors.New("数据库连接池未初始化")
	}

	if config == nil {
		config = DefaultAsyncConfig()
	}

	async := &asyncDB{
		pool:        pool,
		config:      config,
		queue:       make(chan *AsyncOperation, config.QueueSize),
		batchQueues: make(map[string][]*AsyncOperation),
		batchTimers: make(map[string]*time.Timer),
		stopChan:    make(chan struct{}),
	}

	// 启动工作线程
	for i := 0; i < config.WorkerCount; i++ {
		async.wg.Add(1)
		go async.worker(i)
	}

	log.Printf("异步数据库访问已初始化，工作线程数: %d, 队列大小: %d", config.WorkerCount, config.QueueSize)
	return async, nil
}

// QueryAsync 异步执行查询
func (a *asyncDB) QueryAsync(ctx context.Context, sql string, resultHandler func(rows pgx.Rows) (interface{}, error), callback AsyncCallback, args ...interface{}) error {
	if a.isClosed {
		return errors.New("异步数据库访问已关闭")
	}

	// 创建操作
	op := &AsyncOperation{
		Type:          "query",
		SQL:           sql,
		Args:          args,
		ResultHandler: resultHandler,
		Callback:      callback,
		Context:       ctx,
		SubmittedAt:   time.Now(),
	}

	// 如果启用了批处理，尝试将操作添加到批处理队列
	if a.config.EnableBatching && isBatchableQuery(sql) {
		batchID := getBatchID(sql)
		if batchID != "" {
			a.addToBatch(batchID, op)
			return nil
		}
	}

	// 否则，直接添加到操作队列
	return a.enqueueOperation(op)
}

// ExecAsync 异步执行命令
func (a *asyncDB) ExecAsync(ctx context.Context, sql string, callback AsyncCallback, args ...interface{}) error {
	if a.isClosed {
		return errors.New("异步数据库访问已关闭")
	}

	// 创建操作
	op := &AsyncOperation{
		Type:        "exec",
		SQL:         sql,
		Args:        args,
		Callback:    callback,
		Context:     ctx,
		SubmittedAt: time.Now(),
	}

	// 如果启用了批处理，尝试将操作添加到批处理队列
	if a.config.EnableBatching && isBatchableExec(sql) {
		batchID := getBatchID(sql)
		if batchID != "" {
			a.addToBatch(batchID, op)
			return nil
		}
	}

	// 否则，直接添加到操作队列
	return a.enqueueOperation(op)
}

// Stats 获取统计信息
func (a *asyncDB) Stats() *AsyncStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 复制统计信息
	stats := a.stats
	stats.CurrentQueueSize = len(a.queue)

	return &stats
}

// WaitForCompletion 等待所有操作完成
func (a *asyncDB) WaitForCompletion(timeout time.Duration) error {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 创建一个通道，用于等待所有操作完成
	done := make(chan struct{})

	// 启动一个goroutine等待所有操作完成
	go func() {
		// 处理所有批处理队列
		a.processPendingBatches()

		// 检查队列是否为空
		for {
			a.mu.RLock()
			queueLen := len(a.queue)
			pendingBatches := len(a.batchQueues)
			a.mu.RUnlock()

			if queueLen == 0 && pendingBatches == 0 {
				// 队列为空，操作完成
				close(done)
				return
			}

			// 等待一小段时间后再次检查
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// 等待完成或超时
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("等待操作完成超时: %w", ctx.Err())
	}
}

// Close 关闭异步数据库访问
func (a *asyncDB) Close() error {
	a.mu.Lock()
	if a.isClosed {
		a.mu.Unlock()
		return nil
	}
	a.isClosed = true
	a.mu.Unlock()

	// 处理所有批处理队列
	a.processPendingBatches()

	// 关闭停止通道，通知所有工作线程退出
	close(a.stopChan)

	// 等待所有工作线程完成
	a.wg.Wait()

	// 关闭队列
	close(a.queue)

	log.Println("异步数据库访问已关闭")
	return nil
}

// worker 工作线程
func (a *asyncDB) worker(id int) {
	defer a.wg.Done()
	log.Printf("工作线程 %d 已启动", id)

	for {
		select {
		case <-a.stopChan:
			log.Printf("工作线程 %d 收到停止信号，退出", id)
			return
		case op, ok := <-a.queue:
			if !ok {
				log.Printf("工作线程 %d 队列已关闭，退出", id)
				return
			}

			// 处理操作
			a.processOperation(op)
		}
	}
}

// processOperation 处理单个操作
func (a *asyncDB) processOperation(op *AsyncOperation) {
	startTime := time.Now()
	result := &AsyncResult{
		Success: false,
	}

	// 检查上下文是否已取消
	if op.Context.Err() != nil {
		result.Error = fmt.Errorf("操作已取消: %w", op.Context.Err())
		a.completeOperation(op, result, startTime)
		return
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(op.Context, a.config.OperationTimeout)
	defer cancel()

	var err error
	switch op.Type {
	case "query":
		// 执行查询
		var rows pgx.Rows
		rows, err = a.pool.Query(ctx, op.SQL, op.Args...)
		if err == nil && op.ResultHandler != nil {
			// 处理结果
			result.Data, err = op.ResultHandler(rows)
			rows.Close()
		}
	case "exec":
		// 执行命令
		var tag pgconn.CommandTag
		tag, err = a.pool.Exec(ctx, op.SQL, op.Args...)
		if err == nil {
			result.RowsAffected = tag.RowsAffected()
		}
	}

	// 处理错误
	if err != nil {
		// 检查是否需要重试
		if op.RetryCount < a.config.MaxRetries {
			a.mu.Lock()
			a.stats.RetryCount++
			a.mu.Unlock()

			op.RetryCount++
			time.Sleep(a.config.RetryInterval)
			a.enqueueOperation(op)
			return
		}

		result.Error = fmt.Errorf("操作失败: %w", err)
		result.Success = false

		a.mu.Lock()
		a.stats.FailedOperations++
		a.mu.Unlock()
	} else {
		result.Success = true
	}

	// 完成操作
	a.completeOperation(op, result, startTime)
}

// completeOperation 完成操作并调用回调
func (a *asyncDB) completeOperation(op *AsyncOperation, result *AsyncResult, startTime time.Time) {
	// 设置完成时间和持续时间
	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(startTime)

	// 更新统计信息
	a.mu.Lock()
	a.stats.CompletedOperations++
	a.stats.AvgProcessingTimeMs = (a.stats.AvgProcessingTimeMs*float64(a.stats.CompletedOperations-1) + float64(result.Duration.Milliseconds())) / float64(a.stats.CompletedOperations)
	a.mu.Unlock()

	// 调用回调
	if op.Callback != nil {
		go op.Callback(result)
	}
}

// enqueueOperation 将操作添加到队列
func (a *asyncDB) enqueueOperation(op *AsyncOperation) error {
	select {
	case a.queue <- op:
		// 更新统计信息
		a.mu.Lock()
		a.stats.QueuedOperations++
		a.mu.Unlock()
		return nil
	default:
		return errors.New("操作队列已满")
	}
}

// addToBatch 将操作添加到批处理队列
func (a *asyncDB) addToBatch(batchID string, op *AsyncOperation) {
	a.batchMu.Lock()
	defer a.batchMu.Unlock()

	// 设置批处理ID
	op.BatchID = batchID

	// 添加到批处理队列
	a.batchQueues[batchID] = append(a.batchQueues[batchID], op)

	// 如果批处理队列达到批处理大小，立即处理
	if len(a.batchQueues[batchID]) >= a.config.BatchSize {
		a.processBatch(batchID)
		return
	}

	// 否则，设置定时器
	if _, exists := a.batchTimers[batchID]; !exists {
		a.batchTimers[batchID] = time.AfterFunc(a.config.BatchInterval, func() {
			a.processBatchByTimer(batchID)
		})
	}
}

// processBatchByTimer 由定时器触发处理批处理
func (a *asyncDB) processBatchByTimer(batchID string) {
	a.batchMu.Lock()
	defer a.batchMu.Unlock()

	// 删除定时器
	delete(a.batchTimers, batchID)

	// 处理批处理
	a.processBatch(batchID)
}

// processBatch 处理批处理
func (a *asyncDB) processBatch(batchID string) {
	// 获取批处理队列
	batch, exists := a.batchQueues[batchID]
	if !exists || len(batch) == 0 {
		return
	}

	// 删除批处理队列
	delete(a.batchQueues, batchID)

	// 更新统计信息
	a.mu.Lock()
	a.stats.BatchesProcessed++
	a.stats.AvgBatchSizeItems = (a.stats.AvgBatchSizeItems*float64(a.stats.BatchesProcessed-1) + float64(len(batch))) / float64(a.stats.BatchesProcessed)
	a.mu.Unlock()

	// 将批处理中的操作添加到队列
	for _, op := range batch {
		a.enqueueOperation(op)
	}
}

// processPendingBatches 处理所有待处理的批处理
func (a *asyncDB) processPendingBatches() {
	a.batchMu.Lock()
	defer a.batchMu.Unlock()

	// 停止所有定时器
	for _, timer := range a.batchTimers {
		timer.Stop()
	}
	a.batchTimers = make(map[string]*time.Timer)

	// 处理所有批处理
	for batchID := range a.batchQueues {
		a.processBatch(batchID)
	}
}

// isBatchableQuery 检查查询是否可以批处理
func isBatchableQuery(sql string) bool {
	// 简单实现：只有SELECT语句可以批处理
	// 实际实现可能需要更复杂的解析
	return len(sql) > 6 && sql[0:6] == "SELECT"
}

// isBatchableExec 检查执行是否可以批处理
func isBatchableExec(sql string) bool {
	// 简单实现：INSERT, UPDATE, DELETE语句可以批处理
	// 实际实现可能需要更复杂的解析
	if len(sql) < 6 {
		return false
	}
	prefix := sql[0:6]
	return prefix == "INSERT" || prefix == "UPDATE" || prefix == "DELETE"
}

// getBatchID 获取批处理ID
func getBatchID(sql string) string {
	// 简单实现：使用SQL语句的前20个字符作为批处理ID
	// 实际实现可能需要更复杂的解析
	if len(sql) <= 20 {
		return sql
	}
	return sql[0:20]
}
