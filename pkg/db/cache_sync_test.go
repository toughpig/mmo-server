package db

import (
	"context"
	"errors"
	"testing"
	"time"

	redisClient "mmo-server/pkg/redis"

	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRedisCache 模拟Redis缓存接口
type MockRedisCache struct {
	mock.Mock
}

func (m *MockRedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	args := m.Called(ctx, key, dest)
	return args.Error(0)
}

func (m *MockRedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	args := m.Called(ctx, key, value, ttl)
	return args.Error(0)
}

func (m *MockRedisCache) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockRedisCache) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockRedisCache) MGet(ctx context.Context, keys []string) ([]interface{}, error) {
	args := m.Called(ctx, keys)
	return args.Get(0).([]interface{}), args.Error(1)
}

func (m *MockRedisCache) MSet(ctx context.Context, items map[string]interface{}) error {
	args := m.Called(ctx, items)
	return args.Error(0)
}

func (m *MockRedisCache) Incr(ctx context.Context, key string) (int64, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	args := m.Called(ctx, key, ttl)
	return args.Error(0)
}

func (m *MockRedisCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	args := m.Called(ctx, pattern)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockRedisCache) Stats() *redisClient.CacheStats {
	args := m.Called()
	if stats := args.Get(0); stats != nil {
		return stats.(*redisClient.CacheStats)
	}
	return nil
}

// MockAsyncDB 模拟异步数据库接口
type MockAsyncDB struct {
	mock.Mock
}

func (m *MockAsyncDB) QueryAsync(ctx context.Context, sql string, resultHandler func(rows pgx.Rows) (interface{}, error), callback AsyncCallback, args ...interface{}) error {
	mockArgs := m.Called(ctx, sql, resultHandler, callback, args)
	return mockArgs.Error(0)
}

func (m *MockAsyncDB) ExecAsync(ctx context.Context, sql string, callback AsyncCallback, args ...interface{}) error {
	mockArgs := m.Called(ctx, sql, callback, args)
	return mockArgs.Error(0)
}

func (m *MockAsyncDB) Stats() *AsyncStats {
	args := m.Called()
	if stats := args.Get(0); stats != nil {
		return stats.(*AsyncStats)
	}
	return nil
}

func (m *MockAsyncDB) WaitForCompletion(timeout time.Duration) error {
	args := m.Called(timeout)
	return args.Error(0)
}

func (m *MockAsyncDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

// 为测试定义一个Redis错误
var ErrKeyNotFound = errors.New("redis: key not found")

// 测试缓存同步创建
func TestNewCacheSync(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 测试正常创建
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)
	require.NotNil(t, cs)

	// 测试缺少缓存
	cs, err = NewCacheSync(nil, mockDB, config)
	assert.Error(t, err)
	assert.Nil(t, cs)

	// 测试缺少数据库
	cs, err = NewCacheSync(mockCache, nil, config)
	assert.Error(t, err)
	assert.Nil(t, cs)

	// 测试缺少配置（应使用默认配置）
	cs, err = NewCacheSync(mockCache, mockDB, nil)
	require.NoError(t, err)
	require.NotNil(t, cs)
}

// 测试Get方法
func TestCacheSync_Get(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 准备测试数据
	entityType := "user"
	entityID := "123"
	cacheKey := "cache:user:123"
	testData := EntityData{"name": "John", "age": 30}

	// 场景1: 缓存命中
	mockCache.On("Get", mock.Anything, cacheKey, mock.AnythingOfType("*db.EntityData")).
		Run(func(args mock.Arguments) {
			dest := args.Get(2).(*EntityData)
			*dest = testData
		}).
		Return(nil).Once()

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 执行Get操作
	data, err := cs.Get(context.Background(), entityType, entityID)
	require.NoError(t, err)
	assert.Equal(t, testData, data)

	// 场景2: 缓存未命中，数据库命中
	mockCache.On("Get", mock.Anything, cacheKey, mock.AnythingOfType("*db.EntityData")).
		Return(ErrKeyNotFound).Once()

	// 模拟数据库查询
	mockDB.On("QueryAsync", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 调用回调函数模拟数据库返回
			callback := args.Get(3).(AsyncCallback)
			callback(&AsyncResult{
				Success: true,
				Data:    testData,
			})
		}).
		Return(nil).Once()

	// 模拟缓存设置
	mockCache.On("Set", mock.Anything, cacheKey, testData, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, cacheKey, mock.Anything).Return(nil).Once()

	// 执行Get操作
	data, err = cs.Get(context.Background(), entityType, entityID)
	require.NoError(t, err)
	assert.Equal(t, testData, data)

	// 场景3: 缓存未命中，数据库查询失败
	mockCache.On("Get", mock.Anything, cacheKey, mock.AnythingOfType("*db.EntityData")).
		Return(ErrKeyNotFound).Once()

	dbError := errors.New("database error")
	mockDB.On("QueryAsync", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 调用回调函数模拟数据库错误
			callback := args.Get(3).(AsyncCallback)
			callback(&AsyncResult{
				Success: false,
				Error:   dbError,
			})
		}).
		Return(nil).Once()

	// 执行Get操作
	data, err = cs.Get(context.Background(), entityType, entityID)
	assert.Error(t, err)
	assert.Nil(t, data)

	// 确认所有预期的调用都已发生
	mockCache.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

// 测试Set方法
func TestCacheSync_Set(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 准备测试数据
	entityType := "user"
	entityID := "123"
	cacheKey := "cache:user:123"
	testData := EntityData{"name": "John", "age": 30}

	// 场景1: 延迟写入数据库
	mockCache.On("Set", mock.Anything, cacheKey, testData, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, cacheKey, mock.Anything).Return(nil).Once()

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 执行Set操作
	err = cs.Set(context.Background(), entityType, entityID, testData, nil)
	require.NoError(t, err)

	// 场景2: 立即写入数据库
	mockCache.On("Set", mock.Anything, cacheKey, testData, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, cacheKey, mock.Anything).Return(nil).Once()

	mockDB.On("ExecAsync", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 调用回调函数模拟数据库成功
			callback := args.Get(2).(AsyncCallback)
			callback(&AsyncResult{
				Success:      true,
				RowsAffected: 1,
			})
		}).
		Return(nil).Once()

	// 执行Set操作 (立即写入)
	opts := &WriteOptions{Immediate: true}
	err = cs.Set(context.Background(), entityType, entityID, testData, opts)
	require.NoError(t, err)

	// 场景3: 缓存写入失败
	cacheError := errors.New("cache error")
	mockCache.On("Set", mock.Anything, cacheKey, testData, mock.Anything).Return(cacheError).Once()

	// 执行Set操作
	err = cs.Set(context.Background(), entityType, entityID, testData, nil)
	assert.Error(t, err)
	assert.Equal(t, cacheError, err)

	// 确认所有预期的调用都已发生
	mockCache.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

// 测试Delete方法
func TestCacheSync_Delete(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 准备测试数据
	entityType := "user"
	entityID := "123"
	cacheKey := "cache:user:123"

	// 场景1: 延迟删除
	mockCache.On("Delete", mock.Anything, cacheKey).Return(nil).Once()

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 执行Delete操作
	err = cs.Delete(context.Background(), entityType, entityID, nil)
	require.NoError(t, err)

	// 场景2: 立即删除
	mockCache.On("Delete", mock.Anything, cacheKey).Return(nil).Once()

	mockDB.On("ExecAsync", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 调用回调函数模拟数据库成功
			callback := args.Get(2).(AsyncCallback)
			callback(&AsyncResult{
				Success:      true,
				RowsAffected: 1,
			})
		}).
		Return(nil).Once()

	// 执行Delete操作 (立即删除)
	opts := &WriteOptions{Immediate: true}
	err = cs.Delete(context.Background(), entityType, entityID, opts)
	require.NoError(t, err)

	// 场景3: 缓存删除失败
	cacheError := errors.New("cache error")
	mockCache.On("Delete", mock.Anything, cacheKey).Return(cacheError).Once()

	// 执行Delete操作
	err = cs.Delete(context.Background(), entityType, entityID, nil)
	assert.Error(t, err)
	assert.Equal(t, cacheError, err)

	// 确认所有预期的调用都已发生
	mockCache.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

// 测试Flush方法
func TestCacheSync_Flush(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 准备测试数据
	entityType := "user"
	entityID := "123"
	testData := EntityData{"name": "John", "age": 30}

	// 模拟将数据加入脏数据集
	mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 先设置数据（加入脏数据集）
	err = cs.Set(context.Background(), entityType, entityID, testData, nil)
	require.NoError(t, err)

	// 模拟查询脏键
	mockCache.On("Keys", mock.Anything, "dirty:*").Return([]string{"dirty:user:123"}, nil).Once()

	// 模拟获取实体类型和ID
	mockCache.On("Get", mock.Anything, "dirty:user:123", mock.AnythingOfType("*db.DirtyEntry")).
		Run(func(args mock.Arguments) {
			dest := args.Get(2).(*DirtyEntry)
			*dest = DirtyEntry{
				EntityType: entityType,
				EntityID:   entityID,
				Data:       testData,
				Status:     DirtyStatusNew,
			}
		}).
		Return(nil).Once()

	// 模拟刷新脏数据到数据库
	mockDB.On("ExecAsync", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 调用回调函数模拟数据库成功
			callback := args.Get(2).(AsyncCallback)
			callback(&AsyncResult{
				Success:      true,
				RowsAffected: 1,
			})
		}).
		Return(nil).Once()

	// 模拟删除脏键
	mockCache.On("Delete", mock.Anything, "dirty:user:123").Return(nil).Once()

	// 执行Flush操作
	err = cs.Flush(context.Background())
	require.NoError(t, err)

	// 确认所有预期的调用都已发生
	mockCache.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

// 测试GetMulti方法
func TestCacheSync_GetMulti(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 准备测试数据
	entityType := "user"
	entityIDs := []string{"101", "102", "103"}
	cacheKeys := []string{"cache:user:101", "cache:user:102", "cache:user:103"}

	userData1 := EntityData{"name": "User1", "age": 25}
	userData2 := EntityData{"name": "User2", "age": 30}
	userData3 := EntityData{"name": "User3", "age": 35}

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 场景1: 部分缓存命中
	// 模拟MGet调用
	mockCache.On("MGet", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			keys := args.Get(1).([]string)
			assert.ElementsMatch(t, keys, cacheKeys)
		}).
		Return([]interface{}{
			userData1, // 101 在缓存中
			nil,       // 102 不在缓存中
			userData3, // 103 在缓存中
		}, nil).Once()

	// 模拟数据库查询未命中的数据
	mockDB.On("QueryAsync", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 调用回调函数模拟数据库返回
			callback := args.Get(3).(AsyncCallback)
			callback(&AsyncResult{
				Success: true,
				Data:    userData2,
			})
		}).
		Return(nil).Once()

	// 模拟缓存设置
	mockCache.On("Set", mock.Anything, "cache:user:102", userData2, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, "cache:user:102", mock.Anything).Return(nil).Once()

	// 执行GetMulti操作
	result, err := cs.GetMulti(context.Background(), entityType, entityIDs)
	require.NoError(t, err)
	assert.Equal(t, 3, len(result))
	assert.Equal(t, userData1, result["101"])
	assert.Equal(t, userData2, result["102"])
	assert.Equal(t, userData3, result["103"])

	// 确认所有预期的调用都已发生
	mockCache.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

// 测试SetMulti方法
func TestCacheSync_SetMulti(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 准备测试数据
	entityType := "user"
	userData1 := EntityData{"name": "User1", "age": 25}
	userData2 := EntityData{"name": "User2", "age": 30}
	entities := map[string]EntityData{
		"101": userData1,
		"102": userData2,
	}

	// 场景1: 延迟写入数据库
	// 模拟缓存设置
	mockCache.On("Set", mock.Anything, "cache:user:101", userData1, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, "cache:user:101", mock.Anything).Return(nil).Once()
	mockCache.On("Set", mock.Anything, "cache:user:102", userData2, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, "cache:user:102", mock.Anything).Return(nil).Once()

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 执行SetMulti操作
	err = cs.SetMulti(context.Background(), entityType, entities, nil)
	require.NoError(t, err)

	// 场景2: 立即写入数据库
	// 模拟缓存设置
	mockCache.On("Set", mock.Anything, "cache:user:101", userData1, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, "cache:user:101", mock.Anything).Return(nil).Once()
	mockCache.On("Set", mock.Anything, "cache:user:102", userData2, mock.Anything).Return(nil).Once()
	mockCache.On("Expire", mock.Anything, "cache:user:102", mock.Anything).Return(nil).Once()

	// 模拟数据库批量写入 - 使用ExecAsync来处理键值对的写入
	mockDB.On("ExecAsync", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 调用回调函数模拟数据库成功
			callback := args.Get(2).(AsyncCallback)
			callback(&AsyncResult{
				Success:      true,
				RowsAffected: 2,
			})
		}).
		Return(nil).Once()

	// 执行SetMulti操作 (立即写入)
	opts := &WriteOptions{Immediate: true}
	err = cs.SetMulti(context.Background(), entityType, entities, opts)
	require.NoError(t, err)

	// 确认所有预期的调用都已发生
	mockCache.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

// 测试DeleteMulti方法
func TestCacheSync_DeleteMulti(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 准备测试数据
	entityType := "user"
	entityIDs := []string{"101", "102"}

	// 场景1: 延迟删除
	// 模拟缓存删除
	mockCache.On("Delete", mock.Anything, "cache:user:101").Return(nil).Once()
	mockCache.On("Delete", mock.Anything, "cache:user:102").Return(nil).Once()

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 执行DeleteMulti操作
	err = cs.DeleteMulti(context.Background(), entityType, entityIDs, nil)
	require.NoError(t, err)

	// 场景2: 立即删除
	// 模拟缓存删除
	mockCache.On("Delete", mock.Anything, "cache:user:101").Return(nil).Once()
	mockCache.On("Delete", mock.Anything, "cache:user:102").Return(nil).Once()

	// 模拟数据库批量删除
	mockDB.On("ExecAsync", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			// 调用回调函数模拟数据库成功
			callback := args.Get(2).(AsyncCallback)
			callback(&AsyncResult{
				Success:      true,
				RowsAffected: 2,
			})
		}).
		Return(nil).Once()

	// 执行DeleteMulti操作 (立即删除)
	opts := &WriteOptions{Immediate: true}
	err = cs.DeleteMulti(context.Background(), entityType, entityIDs, opts)
	require.NoError(t, err)

	// 确认所有预期的调用都已发生
	mockCache.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

// 测试Close方法
func TestCacheSync_Close(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 模拟数据库关闭
	mockDB.On("WaitForCompletion", mock.Anything).Return(nil).Once()

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 执行Close操作
	err = cs.Close()
	require.NoError(t, err)

	// 确认所有预期的调用都已发生
	mockDB.AssertExpectations(t)
}

// 测试Stats方法
func TestCacheSync_Stats(t *testing.T) {
	mockCache := &MockRedisCache{}
	mockDB := &MockAsyncDB{}
	config := DefaultCacheSyncConfig()

	// 创建缓存同步对象
	cs, err := NewCacheSync(mockCache, mockDB, config)
	require.NoError(t, err)

	// 执行Stats操作
	stats := cs.Stats()
	require.NotNil(t, stats)

	// 默认情况下所有计数应该为0
	assert.Equal(t, int64(0), stats.CacheHits)
	assert.Equal(t, int64(0), stats.CacheMisses)
	assert.Equal(t, 0, stats.DirtyCount)
}
