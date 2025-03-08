package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"mmo-server/pkg/redis"

	"github.com/jackc/pgx/v4"
)

// CacheSyncConfig 缓存同步配置
type CacheSyncConfig struct {
	// 脏数据刷新间隔
	FlushInterval time.Duration
	// 脏数据批量大小
	BatchSize int
	// 脏数据过期时间（超过此时间强制刷新）
	DirtyTTL time.Duration
	// 缓存过期时间
	CacheTTL time.Duration
	// 是否启用批量写入数据库
	EnableBatchWrite bool
	// 是否启用预读取
	EnablePrefetch bool
	// 最大脏数据数量（超过此数量强制刷新）
	MaxDirtySize int
	// 脏数据持久化（为了系统崩溃恢复）
	EnableDirtyPersistence bool
	// 持久化间隔
	PersistenceInterval time.Duration
}

// DefaultCacheSyncConfig 返回默认缓存同步配置
func DefaultCacheSyncConfig() *CacheSyncConfig {
	return &CacheSyncConfig{
		FlushInterval:          30 * time.Second,
		BatchSize:              100,
		DirtyTTL:               5 * time.Minute,
		CacheTTL:               1 * time.Hour,
		EnableBatchWrite:       true,
		EnablePrefetch:         true,
		MaxDirtySize:           1000,
		EnableDirtyPersistence: false,
		PersistenceInterval:    5 * time.Minute,
	}
}

// DirtyStatus 表示脏数据状态
type DirtyStatus string

const (
	// DirtyStatusNew 新创建的脏数据
	DirtyStatusNew DirtyStatus = "new"
	// DirtyStatusPending 等待刷新的脏数据
	DirtyStatusPending DirtyStatus = "pending"
	// DirtyStatusFlushing 正在刷新的脏数据
	DirtyStatusFlushing DirtyStatus = "flushing"
)

// DirtyEntry 脏数据条目
type DirtyEntry struct {
	// 实体类型
	EntityType string
	// 实体ID
	EntityID string
	// 实体数据
	Data interface{}
	// 最后修改时间
	ModifiedAt time.Time
	// 状态
	Status DirtyStatus
	// 版本号（用于冲突检测）
	Version int64
	// 标记（用于批量操作）
	Tags []string
}

// CacheSyncStats 缓存同步统计信息
type CacheSyncStats struct {
	// 缓存命中次数
	CacheHits int64
	// 缓存未命中次数
	CacheMisses int64
	// 脏数据数量
	DirtyCount int
	// 累计刷新次数
	FlushCount int64
	// 累计刷新条目数量
	FlushedEntries int64
	// 平均刷新时间（毫秒）
	AvgFlushTimeMs float64
	// 最后刷新时间
	LastFlushTime time.Time
	// 冲突次数
	ConflictCount int64
}

// EntityData 实体数据的通用表示
type EntityData map[string]interface{}

// WriteOptions 写入选项
type WriteOptions struct {
	// 是否立即写入数据库
	Immediate bool
	// 数据版本（用于冲突检测）
	Version int64
	// 数据标记（用于批量操作）
	Tags []string
}

// CacheSync 缓存同步接口
type CacheSync interface {
	// 获取实体数据（先查缓存，未命中则查数据库并缓存）
	Get(ctx context.Context, entityType string, entityID string) (EntityData, error)
	// 批量获取实体数据
	GetMulti(ctx context.Context, entityType string, entityIDs []string) (map[string]EntityData, error)
	// 存储实体数据（先写缓存，定期刷新到数据库）
	Set(ctx context.Context, entityType string, entityID string, data EntityData, opts *WriteOptions) error
	// 批量存储实体数据
	SetMulti(ctx context.Context, entityType string, entities map[string]EntityData, opts *WriteOptions) error
	// 删除实体数据
	Delete(ctx context.Context, entityType string, entityID string, opts *WriteOptions) error
	// 批量删除实体数据
	DeleteMulti(ctx context.Context, entityType string, entityIDs []string, opts *WriteOptions) error
	// 获取统计信息
	Stats() *CacheSyncStats
	// 手动触发刷新操作
	Flush(ctx context.Context) error
	// 关闭
	Close() error
}

// cacheSync 实现缓存同步
type cacheSync struct {
	config       *CacheSyncConfig
	cache        redis.Cache
	asyncDB      AsyncDB
	dirtyEntries map[string]*DirtyEntry
	mu           sync.RWMutex
	stats        CacheSyncStats
	stopChan     chan struct{}
	flushTicker  *time.Ticker
	isClosed     bool
}

// NewCacheSync 创建缓存同步
func NewCacheSync(cache redis.Cache, asyncDB AsyncDB, config *CacheSyncConfig) (CacheSync, error) {
	if cache == nil {
		return nil, errors.New("缓存未初始化")
	}
	if asyncDB == nil {
		return nil, errors.New("异步数据库未初始化")
	}
	if config == nil {
		config = DefaultCacheSyncConfig()
	}

	cs := &cacheSync{
		config:       config,
		cache:        cache,
		asyncDB:      asyncDB,
		dirtyEntries: make(map[string]*DirtyEntry),
		stopChan:     make(chan struct{}),
	}

	// 启动定期刷新任务
	cs.flushTicker = time.NewTicker(config.FlushInterval)
	go cs.flushLoop()

	log.Printf("缓存同步已初始化, 刷新间隔: %v, 最大脏数据: %d",
		config.FlushInterval, config.MaxDirtySize)
	return cs, nil
}

// Get 获取实体数据
func (cs *cacheSync) Get(ctx context.Context, entityType string, entityID string) (EntityData, error) {
	cacheKey := cs.formatCacheKey(entityType, entityID)

	// 先检查缓存
	var data EntityData
	err := cs.cache.Get(ctx, cacheKey, &data)
	if err == nil {
		// 缓存命中
		cs.mu.Lock()
		cs.stats.CacheHits++
		cs.mu.Unlock()
		return data, nil
	}

	// 缓存未命中，检查脏数据
	cs.mu.RLock()
	dirtyKey := cs.formatDirtyKey(entityType, entityID)
	dirtyEntry, existsInDirty := cs.dirtyEntries[dirtyKey]
	cs.mu.RUnlock()

	if existsInDirty {
		// 从脏数据返回
		if dirtyData, ok := dirtyEntry.Data.(EntityData); ok {
			// 缓存数据以便后续访问
			cs.cache.Set(ctx, cacheKey, dirtyData, cs.config.CacheTTL)
			return dirtyData, nil
		}
	}

	// 记录缓存未命中
	cs.mu.Lock()
	cs.stats.CacheMisses++
	cs.mu.Unlock()

	// 从数据库查询
	var entity EntityData
	resultChan := make(chan struct{})
	var queryErr error

	err = cs.asyncDB.QueryAsync(ctx, fmt.Sprintf("SELECT * FROM %s WHERE id = $1", entityType),
		func(rows pgx.Rows) (interface{}, error) {
			if !rows.Next() {
				return nil, fmt.Errorf("实体 %s:%s 不存在", entityType, entityID)
			}

			// 扫描结果到map
			cols := rows.FieldDescriptions()
			vals := make([]interface{}, len(cols))
			valPtrs := make([]interface{}, len(cols))

			for i := range vals {
				valPtrs[i] = &vals[i]
			}

			if err := rows.Scan(valPtrs...); err != nil {
				return nil, err
			}

			// 构建实体数据
			entity = make(EntityData)
			for i, col := range cols {
				entity[string(col.Name)] = vals[i]
			}

			return entity, nil
		},
		func(result *AsyncResult) {
			defer close(resultChan)

			if !result.Success {
				queryErr = result.Error
				return
			}

			if result.Data != nil {
				entity = result.Data.(EntityData)
				// 缓存结果
				cs.cache.Set(ctx, cacheKey, entity, cs.config.CacheTTL)
			}
		},
		entityID)

	if err != nil {
		return nil, fmt.Errorf("查询实体失败: %w", err)
	}

	// 等待查询完成
	<-resultChan

	if queryErr != nil {
		return nil, fmt.Errorf("查询实体失败: %w", queryErr)
	}

	return entity, nil
}

// GetMulti 批量获取实体数据
func (cs *cacheSync) GetMulti(ctx context.Context, entityType string, entityIDs []string) (map[string]EntityData, error) {
	if len(entityIDs) == 0 {
		return make(map[string]EntityData), nil
	}

	result := make(map[string]EntityData)
	missingIDs := make([]string, 0, len(entityIDs))

	// 批量查询缓存
	cacheKeys := make([]string, len(entityIDs))
	idToKey := make(map[string]string, len(entityIDs))

	for i, id := range entityIDs {
		cacheKey := cs.formatCacheKey(entityType, id)
		cacheKeys[i] = cacheKey
		idToKey[id] = cacheKey
	}

	// 从缓存批量获取
	cachedData, err := cs.cache.MGet(ctx, cacheKeys)
	if err != nil {
		// 忽略缓存错误，继续查询数据库
		log.Printf("批量查询缓存失败: %v", err)
	} else {
		// 处理缓存结果
		for i, data := range cachedData {
			if data != nil {
				// 缓存命中
				var entityData EntityData
				if jsonStr, ok := data.(string); ok {
					if err := json.Unmarshal([]byte(jsonStr), &entityData); err == nil {
						id := entityIDs[i]
						result[id] = entityData

						cs.mu.Lock()
						cs.stats.CacheHits++
						cs.mu.Unlock()

						continue
					}
				}
			}

			// 缓存未命中
			missingIDs = append(missingIDs, entityIDs[i])

			cs.mu.Lock()
			cs.stats.CacheMisses++
			cs.mu.Unlock()
		}
	}

	// 检查脏数据
	cs.mu.RLock()
	for _, id := range missingIDs {
		dirtyKey := cs.formatDirtyKey(entityType, id)
		if dirtyEntry, exists := cs.dirtyEntries[dirtyKey]; exists {
			if dirtyData, ok := dirtyEntry.Data.(EntityData); ok {
				result[id] = dirtyData

				// 从missingIDs中移除
				for i, missingID := range missingIDs {
					if missingID == id {
						missingIDs = append(missingIDs[:i], missingIDs[i+1:]...)
						break
					}
				}

				// 缓存数据以便后续访问
				cacheKey := cs.formatCacheKey(entityType, id)
				cs.cache.Set(ctx, cacheKey, dirtyData, cs.config.CacheTTL)
			}
		}
	}
	cs.mu.RUnlock()

	// 如果所有ID都已获取，直接返回
	if len(missingIDs) == 0 {
		return result, nil
	}

	// 构建SQL占位符
	placeholders := make([]string, len(missingIDs))
	args := make([]interface{}, len(missingIDs))

	for i, id := range missingIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	// 构建SQL查询
	query := fmt.Sprintf("SELECT * FROM %s WHERE id IN (%s)",
		entityType, strings.Join(placeholders, ", "))

	// 从数据库查询
	resultChan := make(chan struct{})
	var queryErr error

	err = cs.asyncDB.QueryAsync(ctx, query,
		func(rows pgx.Rows) (interface{}, error) {
			dbResult := make(map[string]EntityData)

			for rows.Next() {
				// 扫描结果到map
				cols := rows.FieldDescriptions()
				vals := make([]interface{}, len(cols))
				valPtrs := make([]interface{}, len(cols))

				for i := range vals {
					valPtrs[i] = &vals[i]
				}

				if err := rows.Scan(valPtrs...); err != nil {
					return nil, err
				}

				// 构建实体数据
				entity := make(EntityData)
				var id string

				for i, col := range cols {
					colName := string(col.Name)
					entity[colName] = vals[i]

					if colName == "id" {
						if idVal, ok := vals[i].(string); ok {
							id = idVal
						}
					}
				}

				if id != "" {
					dbResult[id] = entity
				}
			}

			return dbResult, nil
		},
		func(asyncResult *AsyncResult) {
			defer close(resultChan)

			if !asyncResult.Success {
				queryErr = asyncResult.Error
				return
			}

			if asyncResult.Data != nil {
				dbData := asyncResult.Data.(map[string]EntityData)

				// 合并结果
				for id, entity := range dbData {
					result[id] = entity

					// 缓存结果
					cacheKey := cs.formatCacheKey(entityType, id)
					cs.cache.Set(ctx, cacheKey, entity, cs.config.CacheTTL)
				}
			}
		},
		args...)

	if err != nil {
		return result, fmt.Errorf("批量查询实体失败: %w", err)
	}

	// 等待查询完成
	<-resultChan

	if queryErr != nil {
		return result, fmt.Errorf("批量查询实体失败: %w", queryErr)
	}

	return result, nil
}

// Set 存储实体数据
func (cs *cacheSync) Set(ctx context.Context, entityType string, entityID string, data EntityData, opts *WriteOptions) error {
	if opts == nil {
		opts = &WriteOptions{}
	}

	// 生成脏数据条目
	dirtyKey := cs.formatDirtyKey(entityType, entityID)
	dirtyEntry := &DirtyEntry{
		EntityType: entityType,
		EntityID:   entityID,
		Data:       data,
		ModifiedAt: time.Now(),
		Status:     DirtyStatusNew,
		Version:    opts.Version,
		Tags:       opts.Tags,
	}

	// 写入缓存
	cacheKey := cs.formatCacheKey(entityType, entityID)
	if err := cs.cache.Set(ctx, cacheKey, data, cs.config.CacheTTL); err != nil {
		return fmt.Errorf("写入缓存失败: %w", err)
	}

	// 如果需要立即写入数据库
	if opts.Immediate {
		return cs.flushEntry(ctx, dirtyEntry)
	}

	// 否则，添加到脏数据集
	cs.mu.Lock()
	cs.dirtyEntries[dirtyKey] = dirtyEntry
	dirtyCount := len(cs.dirtyEntries)
	cs.mu.Unlock()

	// 如果脏数据数量超过最大值，触发刷新
	if dirtyCount >= cs.config.MaxDirtySize {
		go cs.Flush(context.Background())
	}

	return nil
}

// SetMulti 批量存储实体数据
func (cs *cacheSync) SetMulti(ctx context.Context, entityType string, entities map[string]EntityData, opts *WriteOptions) error {
	if len(entities) == 0 {
		return nil
	}

	if opts == nil {
		opts = &WriteOptions{}
	}

	// 批量写入缓存
	cacheItems := make(map[string]interface{}, len(entities))
	for id, data := range entities {
		cacheKey := cs.formatCacheKey(entityType, id)
		cacheItems[cacheKey] = data
	}

	if err := cs.cache.MSet(ctx, cacheItems); err != nil {
		return fmt.Errorf("批量写入缓存失败: %w", err)
	}

	// 添加到脏数据集或立即写入数据库
	if opts.Immediate {
		// 构建批量更新
		return cs.flushEntities(ctx, entityType, entities)
	} else {
		cs.mu.Lock()
		for id, data := range entities {
			dirtyKey := cs.formatDirtyKey(entityType, id)
			cs.dirtyEntries[dirtyKey] = &DirtyEntry{
				EntityType: entityType,
				EntityID:   id,
				Data:       data,
				ModifiedAt: time.Now(),
				Status:     DirtyStatusNew,
				Version:    opts.Version,
				Tags:       opts.Tags,
			}
		}
		dirtyCount := len(cs.dirtyEntries)
		cs.mu.Unlock()

		// 如果脏数据数量超过最大值，触发刷新
		if dirtyCount >= cs.config.MaxDirtySize {
			go cs.Flush(context.Background())
		}
	}

	return nil
}

// Delete 删除实体数据
func (cs *cacheSync) Delete(ctx context.Context, entityType string, entityID string, opts *WriteOptions) error {
	if opts == nil {
		opts = &WriteOptions{}
	}

	// 从缓存中删除
	cacheKey := cs.formatCacheKey(entityType, entityID)
	if err := cs.cache.Delete(ctx, cacheKey); err != nil {
		// 忽略缓存删除错误
		log.Printf("从缓存删除实体失败: %v", err)
	}

	// 如果需要立即写入数据库
	if opts.Immediate {
		return cs.executeDelete(ctx, entityType, entityID)
	}

	// 否则，添加到脏数据集
	cs.mu.Lock()
	dirtyKey := cs.formatDirtyKey(entityType, entityID)
	cs.dirtyEntries[dirtyKey] = &DirtyEntry{
		EntityType: entityType,
		EntityID:   entityID,
		Data:       nil, // nil表示删除
		ModifiedAt: time.Now(),
		Status:     DirtyStatusNew,
		Version:    opts.Version,
		Tags:       opts.Tags,
	}
	cs.mu.Unlock()

	return nil
}

// DeleteMulti 批量删除实体数据
func (cs *cacheSync) DeleteMulti(ctx context.Context, entityType string, entityIDs []string, opts *WriteOptions) error {
	if len(entityIDs) == 0 {
		return nil
	}

	if opts == nil {
		opts = &WriteOptions{}
	}

	// 批量从缓存中删除
	cacheKeys := make([]string, len(entityIDs))
	for i, id := range entityIDs {
		cacheKeys[i] = cs.formatCacheKey(entityType, id)
	}

	// 从缓存中删除
	for _, key := range cacheKeys {
		cs.cache.Delete(ctx, key)
	}

	// 如果需要立即写入数据库
	if opts.Immediate {
		return cs.executeDeleteMulti(ctx, entityType, entityIDs)
	}

	// 否则，添加到脏数据集
	cs.mu.Lock()
	for _, id := range entityIDs {
		dirtyKey := cs.formatDirtyKey(entityType, id)
		cs.dirtyEntries[dirtyKey] = &DirtyEntry{
			EntityType: entityType,
			EntityID:   id,
			Data:       nil, // nil表示删除
			ModifiedAt: time.Now(),
			Status:     DirtyStatusNew,
			Version:    opts.Version,
			Tags:       opts.Tags,
		}
	}
	cs.mu.Unlock()

	return nil
}

// Stats 获取统计信息
func (cs *cacheSync) Stats() *CacheSyncStats {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// 复制统计信息
	stats := cs.stats
	stats.DirtyCount = len(cs.dirtyEntries)

	return &stats
}

// Flush 手动触发刷新操作
func (cs *cacheSync) Flush(ctx context.Context) error {
	startTime := time.Now()

	log.Println("手动触发缓存刷新操作")

	// 获取需要刷新的条目
	cs.mu.Lock()
	entriesToFlush := make([]*DirtyEntry, 0, len(cs.dirtyEntries))

	for _, entry := range cs.dirtyEntries {
		if entry.Status == DirtyStatusNew {
			entry.Status = DirtyStatusPending
			entriesToFlush = append(entriesToFlush, entry)
		}
	}
	cs.mu.Unlock()

	// 如果没有需要刷新的条目，直接返回
	if len(entriesToFlush) == 0 {
		return nil
	}

	// 按实体类型分组
	entriesByType := make(map[string][]*DirtyEntry)
	for _, entry := range entriesToFlush {
		entriesByType[entry.EntityType] = append(entriesByType[entry.EntityType], entry)
	}

	// 处理每种实体类型
	var errs []error
	for entityType, entries := range entriesByType {
		// 计算批次数
		batchCount := (len(entries) + cs.config.BatchSize - 1) / cs.config.BatchSize

		for i := 0; i < batchCount; i++ {
			start := i * cs.config.BatchSize
			end := start + cs.config.BatchSize
			if end > len(entries) {
				end = len(entries)
			}

			batchEntries := entries[start:end]
			if err := cs.flushBatch(ctx, entityType, batchEntries); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// 更新统计信息
	duration := time.Since(startTime)
	cs.mu.Lock()
	cs.stats.FlushCount++
	cs.stats.FlushedEntries += int64(len(entriesToFlush))
	cs.stats.AvgFlushTimeMs = (cs.stats.AvgFlushTimeMs*float64(cs.stats.FlushCount-1) + float64(duration.Milliseconds())) / float64(cs.stats.FlushCount)
	cs.stats.LastFlushTime = time.Now()
	cs.mu.Unlock()

	// 如果有错误，返回第一个错误
	if len(errs) > 0 {
		return errs[0]
	}

	log.Printf("缓存刷新完成, 刷新条目数: %d, 耗时: %v", len(entriesToFlush), duration)
	return nil
}

// Close 关闭缓存同步
func (cs *cacheSync) Close() error {
	cs.mu.Lock()
	if cs.isClosed {
		cs.mu.Unlock()
		return nil
	}
	cs.isClosed = true
	cs.mu.Unlock()

	// 停止刷新定时器
	cs.flushTicker.Stop()

	// 关闭停止通道
	close(cs.stopChan)

	// 执行最后一次刷新
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cs.Flush(ctx); err != nil {
		log.Printf("关闭时执行最后一次刷新失败: %v", err)
	}

	log.Println("缓存同步已关闭")
	return nil
}

// 内部辅助方法

// flushLoop 定期刷新脏数据
func (cs *cacheSync) flushLoop() {
	for {
		select {
		case <-cs.flushTicker.C:
			// 定期刷新
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := cs.Flush(ctx); err != nil {
				log.Printf("定期刷新失败: %v", err)
			}
			cancel()
		case <-cs.stopChan:
			// 停止刷新
			return
		}
	}
}

// flushBatch 刷新一批脏数据
func (cs *cacheSync) flushBatch(ctx context.Context, entityType string, entries []*DirtyEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// 分离插入/更新和删除操作
	updateEntries := make([]*DirtyEntry, 0)
	deleteIDs := make([]string, 0)

	for _, entry := range entries {
		if entry.Data != nil {
			updateEntries = append(updateEntries, entry)
		} else {
			deleteIDs = append(deleteIDs, entry.EntityID)
		}
	}

	// 处理更新操作
	if len(updateEntries) > 0 {
		updateData := make(map[string]EntityData, len(updateEntries))
		for _, entry := range updateEntries {
			updateData[entry.EntityID] = entry.Data.(EntityData)
		}

		if err := cs.flushEntities(ctx, entityType, updateData); err != nil {
			return err
		}
	}

	// 处理删除操作
	if len(deleteIDs) > 0 {
		if err := cs.executeDeleteMulti(ctx, entityType, deleteIDs); err != nil {
			return err
		}
	}

	// 更新脏数据状态
	cs.mu.Lock()
	for _, entry := range entries {
		dirtyKey := cs.formatDirtyKey(entry.EntityType, entry.EntityID)
		delete(cs.dirtyEntries, dirtyKey)
	}
	cs.mu.Unlock()

	return nil
}

// flushEntities 刷新实体数据到数据库
func (cs *cacheSync) flushEntities(ctx context.Context, entityType string, entities map[string]EntityData) error {
	for id, data := range entities {
		// 为每个实体构建SQL更新语句
		fields := make([]string, 0, len(data))
		values := make([]interface{}, 0, len(data)+1)
		placeholders := make([]string, 0, len(data))

		i := 1
		for field, value := range data {
			if field != "id" { // 忽略ID字段
				fields = append(fields, field)
				values = append(values, value)
				placeholders = append(placeholders, fmt.Sprintf("$%d", i))
				i++
			}
		}

		// 添加WHERE条件的参数
		values = append(values, id)

		// 构建SQL更新语句
		sql := fmt.Sprintf("UPDATE %s SET %s = %s WHERE id = $%d",
			entityType,
			strings.Join(fields, ", "),
			strings.Join(placeholders, ", "),
			i)

		resultChan := make(chan struct{})
		var execErr error

		err := cs.asyncDB.ExecAsync(ctx, sql,
			func(result *AsyncResult) {
				defer close(resultChan)

				if !result.Success {
					execErr = result.Error
				}
			},
			values...)

		if err != nil {
			return fmt.Errorf("执行更新失败: %w", err)
		}

		// 等待执行完成
		<-resultChan

		if execErr != nil {
			return fmt.Errorf("更新实体失败: %w", execErr)
		}
	}

	return nil
}

// flushEntry 刷新单个脏数据条目
func (cs *cacheSync) flushEntry(ctx context.Context, entry *DirtyEntry) error {
	if entry.Data == nil {
		// 删除操作
		return cs.executeDelete(ctx, entry.EntityType, entry.EntityID)
	} else {
		// 更新操作
		data, ok := entry.Data.(EntityData)
		if !ok {
			return fmt.Errorf("无效的实体数据类型")
		}

		entities := map[string]EntityData{
			entry.EntityID: data,
		}

		return cs.flushEntities(ctx, entry.EntityType, entities)
	}
}

// executeDelete 执行删除操作
func (cs *cacheSync) executeDelete(ctx context.Context, entityType string, entityID string) error {
	sql := fmt.Sprintf("DELETE FROM %s WHERE id = $1", entityType)

	resultChan := make(chan struct{})
	var execErr error

	err := cs.asyncDB.ExecAsync(ctx, sql,
		func(result *AsyncResult) {
			defer close(resultChan)

			if !result.Success {
				execErr = result.Error
			}
		},
		entityID)

	if err != nil {
		return fmt.Errorf("执行删除失败: %w", err)
	}

	// 等待执行完成
	<-resultChan

	if execErr != nil {
		return fmt.Errorf("删除实体失败: %w", execErr)
	}

	return nil
}

// executeDeleteMulti 执行批量删除操作
func (cs *cacheSync) executeDeleteMulti(ctx context.Context, entityType string, entityIDs []string) error {
	if len(entityIDs) == 0 {
		return nil
	}

	// 构建SQL占位符
	placeholders := make([]string, len(entityIDs))
	args := make([]interface{}, len(entityIDs))

	for i, id := range entityIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)",
		entityType, strings.Join(placeholders, ", "))

	resultChan := make(chan struct{})
	var execErr error

	err := cs.asyncDB.ExecAsync(ctx, sql,
		func(result *AsyncResult) {
			defer close(resultChan)

			if !result.Success {
				execErr = result.Error
			}
		},
		args...)

	if err != nil {
		return fmt.Errorf("执行批量删除失败: %w", err)
	}

	// 等待执行完成
	<-resultChan

	if execErr != nil {
		return fmt.Errorf("批量删除实体失败: %w", execErr)
	}

	return nil
}

// formatCacheKey 格式化缓存键
func (cs *cacheSync) formatCacheKey(entityType string, entityID string) string {
	return fmt.Sprintf("entity:%s:%s", entityType, entityID)
}

// formatDirtyKey 格式化脏数据键
func (cs *cacheSync) formatDirtyKey(entityType string, entityID string) string {
	return fmt.Sprintf("%s:%s", entityType, entityID)
}
