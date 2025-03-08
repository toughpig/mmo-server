package aoi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 测试实体实现
type TestEntity struct {
	id      string
	x, y, z float32
	typeID  int
}

func (e *TestEntity) GetID() string {
	return e.id
}

func (e *TestEntity) GetPosition() (float32, float32, float32) {
	return e.x, e.y, e.z
}

func (e *TestEntity) GetTypeID() int {
	return e.typeID
}

// 更新实体位置
func (e *TestEntity) SetPosition(x, y, z float32) {
	e.x, e.y, e.z = x, y, z
}

// TestNewAOIManager 测试AOI管理器创建
func TestNewAOIManager(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	assert.NotNil(t, aoi, "AOI管理器不应为nil")
	assert.Equal(t, float32(0), aoi.minX, "minX应为0")
	assert.Equal(t, float32(1000), aoi.maxX, "maxX应为1000")
	assert.Equal(t, float32(0), aoi.minZ, "minZ应为0")
	assert.Equal(t, float32(1000), aoi.maxZ, "maxZ应为1000")
	assert.Equal(t, 10, aoi.gridCount, "gridCount应为10")
	assert.Equal(t, float32(100), aoi.gridWidth, "gridWidth应为100")

	// 验证网格初始化
	assert.Equal(t, 10, len(aoi.grids), "网格X方向长度应为10")
	assert.Equal(t, 10, len(aoi.grids[0]), "网格Y方向长度应为10")

	// 验证网格坐标
	assert.Equal(t, 0, aoi.grids[0][0].gridPos.X, "网格(0,0)的X坐标应为0")
	assert.Equal(t, 0, aoi.grids[0][0].gridPos.Y, "网格(0,0)的Y坐标应为0")
	assert.Equal(t, 9, aoi.grids[9][9].gridPos.X, "网格(9,9)的X坐标应为9")
	assert.Equal(t, 9, aoi.grids[9][9].gridPos.Y, "网格(9,9)的Y坐标应为9")
}

// TestAddEntity 测试添加实体
func TestAddEntity(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	// 创建测试实体
	entity := &TestEntity{
		id:     "test1",
		x:      150,
		y:      0,
		z:      250,
		typeID: 0, // 玩家类型
	}

	// 添加实体
	err := aoi.AddEntity(entity)
	assert.NoError(t, err, "添加实体不应返回错误")

	// 验证实体是否在正确的网格中
	grid, err := aoi.GetGrid(entity.x, entity.z)
	assert.NoError(t, err, "获取网格不应返回错误")

	grid.mutex.RLock()
	assert.Equal(t, 1, len(grid.entities), "网格中应该有1个实体")
	assert.Equal(t, entity, grid.entities["test1"], "网格中应该包含添加的实体")
	assert.Equal(t, 1, grid.playerCount, "网格中玩家数量应为1")
	grid.mutex.RUnlock()

	// 测试边界情况
	entityOutside := &TestEntity{
		id:     "outside",
		x:      -10,
		y:      0,
		z:      -10,
		typeID: 0,
	}

	err = aoi.AddEntity(entityOutside)
	assert.Error(t, err, "添加场景外的实体应该返回错误")
}

// TestRemoveEntity 测试移除实体
func TestRemoveEntity(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	// 创建并添加测试实体
	entity := &TestEntity{
		id:     "test1",
		x:      150,
		y:      0,
		z:      250,
		typeID: 0, // 玩家类型
	}

	err := aoi.AddEntity(entity)
	assert.NoError(t, err, "添加实体不应返回错误")

	// 移除实体
	err = aoi.RemoveEntity(entity)
	assert.NoError(t, err, "移除实体不应返回错误")

	// 验证实体是否已被移除
	grid, err := aoi.GetGrid(entity.x, entity.z)
	assert.NoError(t, err, "获取网格不应返回错误")

	grid.mutex.RLock()
	assert.Equal(t, 0, len(grid.entities), "网格中不应该有实体")
	assert.Equal(t, 0, grid.playerCount, "网格中玩家数量应为0")
	grid.mutex.RUnlock()
}

// TestUpdateEntity 测试更新实体位置
func TestUpdateEntity(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	// 创建并添加测试实体
	entity := &TestEntity{
		id:     "test1",
		x:      150,
		y:      0,
		z:      250,
		typeID: 0, // 玩家类型
	}

	err := aoi.AddEntity(entity)
	assert.NoError(t, err, "添加实体不应返回错误")

	// 保存旧位置
	oldX, _, oldZ := entity.GetPosition()

	// 更新实体位置到新网格
	entity.SetPosition(550, 0, 650)

	// 更新AOI中的位置
	err = aoi.UpdateEntity(entity, oldX, oldZ)
	assert.NoError(t, err, "更新实体位置不应返回错误")

	// 验证旧网格中实体已移除
	oldGrid, err := aoi.GetGrid(oldX, oldZ)
	assert.NoError(t, err, "获取旧网格不应返回错误")

	oldGrid.mutex.RLock()
	assert.Equal(t, 0, len(oldGrid.entities), "旧网格中不应该有实体")
	assert.Equal(t, 0, oldGrid.playerCount, "旧网格中玩家数量应为0")
	oldGrid.mutex.RUnlock()

	// 验证新网格中实体已添加
	newGrid, err := aoi.GetGrid(entity.x, entity.z)
	assert.NoError(t, err, "获取新网格不应返回错误")

	newGrid.mutex.RLock()
	assert.Equal(t, 1, len(newGrid.entities), "新网格中应该有1个实体")
	assert.Equal(t, entity, newGrid.entities["test1"], "新网格中应该包含更新的实体")
	assert.Equal(t, 1, newGrid.playerCount, "新网格中玩家数量应为1")
	newGrid.mutex.RUnlock()

	// 测试更新到相同网格
	oldX, _, oldZ = entity.GetPosition()
	entity.SetPosition(560, 0, 660) // 仍在同一网格内

	err = aoi.UpdateEntity(entity, oldX, oldZ)
	assert.NoError(t, err, "更新到相同网格不应返回错误")
}

// TestGetSurroundingGrids 测试获取周围网格
func TestGetSurroundingGrids(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	// 获取中心位置周围网格
	grids, err := aoi.GetSurroundingGrids(500, 500, 1)
	assert.NoError(t, err, "获取周围网格不应返回错误")
	assert.Equal(t, 9, len(grids), "应该返回9个周围网格")

	// 获取边缘位置周围网格
	grids, err = aoi.GetSurroundingGrids(50, 50, 1)
	assert.NoError(t, err, "获取边缘周围网格不应返回错误")
	assert.Equal(t, 4, len(grids), "边缘应该返回4个周围网格")

	// 测试范围参数
	grids, err = aoi.GetSurroundingGrids(500, 500, 2)
	assert.NoError(t, err, "获取大范围周围网格不应返回错误")
	assert.Equal(t, 25, len(grids), "范围2应该返回25个周围网格")
}

// TestGetSurroundingEntities 测试获取周围实体
func TestGetSurroundingEntities(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	// 添加一些测试实体
	entities := []*TestEntity{
		{id: "e1", x: 150, y: 0, z: 150, typeID: 0},
		{id: "e2", x: 250, y: 0, z: 150, typeID: 0},
		{id: "e3", x: 550, y: 0, z: 550, typeID: 1},
		{id: "e4", x: 850, y: 0, z: 850, typeID: 1},
	}

	for _, e := range entities {
		err := aoi.AddEntity(e)
		assert.NoError(t, err, "添加实体不应返回错误")
	}

	// 获取中心点周围实体
	surroundingEntities, err := aoi.GetSurroundingEntities(200, 200, 1)
	assert.NoError(t, err, "获取周围实体不应返回错误")
	assert.Equal(t, 2, len(surroundingEntities), "应该找到2个周围实体")

	// 检查是否包含预期的实体
	foundE1 := false
	foundE2 := false
	for _, e := range surroundingEntities {
		if e.GetID() == "e1" {
			foundE1 = true
		}
		if e.GetID() == "e2" {
			foundE2 = true
		}
	}
	assert.True(t, foundE1, "应该找到实体e1")
	assert.True(t, foundE2, "应该找到实体e2")

	// 测试边缘情况
	surroundingEntities, err = aoi.GetSurroundingEntities(900, 900, 1)
	assert.NoError(t, err, "获取边缘周围实体不应返回错误")
	assert.Equal(t, 1, len(surroundingEntities), "应该找到1个周围实体")
	assert.Equal(t, "e4", surroundingEntities[0].GetID(), "应该找到实体e4")
}

// TestGetEntitiesByDistance 测试根据距离获取实体
func TestGetEntitiesByDistance(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	// 添加一些测试实体
	entities := []*TestEntity{
		{id: "e1", x: 100, y: 0, z: 100, typeID: 0},
		{id: "e2", x: 200, y: 0, z: 200, typeID: 0},
		{id: "e3", x: 300, y: 0, z: 300, typeID: 1},
		{id: "e4", x: 400, y: 0, z: 400, typeID: 1},
	}

	for _, e := range entities {
		err := aoi.AddEntity(e)
		assert.NoError(t, err, "添加实体不应返回错误")
	}

	// 测试距离过滤
	nearbyEntities, err := aoi.GetEntitiesByDistance(200, 0, 200, 150)
	assert.NoError(t, err, "根据距离获取实体不应返回错误")
	assert.Equal(t, 3, len(nearbyEntities), "应该找到3个附近实体")

	// 检查是否包含预期的实体
	foundE1 := false
	foundE2 := false
	for _, e := range nearbyEntities {
		if e.GetID() == "e1" {
			foundE1 = true
		}
		if e.GetID() == "e2" {
			foundE2 = true
		}
	}
	assert.True(t, foundE2, "应该找到实体e2")
	assert.True(t, foundE1, "应该找到实体e1")

	// 测试更大距离
	nearbyEntities, err = aoi.GetEntitiesByDistance(200, 0, 200, 250)
	assert.NoError(t, err, "根据大距离获取实体不应返回错误")
	assert.Equal(t, 3, len(nearbyEntities), "应该找到3个附近实体")
}

// TestGetNearbyPlayers 测试获取附近玩家数量
func TestGetNearbyPlayers(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	// 添加一些测试实体，包括玩家和非玩家
	entities := []*TestEntity{
		{id: "p1", x: 150, y: 0, z: 150, typeID: 0}, // 玩家
		{id: "p2", x: 250, y: 0, z: 150, typeID: 0}, // 玩家
		{id: "m1", x: 550, y: 0, z: 550, typeID: 1}, // 非玩家
		{id: "p3", x: 850, y: 0, z: 850, typeID: 0}, // 玩家
	}

	for _, e := range entities {
		err := aoi.AddEntity(e)
		assert.NoError(t, err, "添加实体不应返回错误")
	}

	// 测试获取附近玩家数量
	playerCount, err := aoi.GetNearbyPlayers(200, 200, 1)
	assert.NoError(t, err, "获取附近玩家数量不应返回错误")
	assert.Equal(t, 2, playerCount, "附近应该有2个玩家")

	// 测试更大范围
	playerCount, err = aoi.GetNearbyPlayers(500, 500, 5)
	assert.NoError(t, err, "获取大范围附近玩家数量不应返回错误")
	assert.Equal(t, 3, playerCount, "大范围内应该有3个玩家")
}

// TestGetGridPosition 测试获取网格位置
func TestGetGridPosition(t *testing.T) {
	aoi := NewAOIManager(0, 1000, 0, 1000, 10)

	// 测试常规坐标的网格位置
	pos, err := aoi.GetGridPosition(150, 250)
	assert.NoError(t, err, "获取网格位置不应返回错误")
	assert.Equal(t, 1, pos.X, "X网格坐标应为1")
	assert.Equal(t, 2, pos.Y, "Y网格坐标应为2")

	// 测试边界坐标
	pos, err = aoi.GetGridPosition(0, 0)
	assert.NoError(t, err, "获取边界网格位置不应返回错误")
	assert.Equal(t, 0, pos.X, "边界X网格坐标应为0")
	assert.Equal(t, 0, pos.Y, "边界Y网格坐标应为0")

	pos, err = aoi.GetGridPosition(999, 999)
	assert.NoError(t, err, "获取边界网格位置不应返回错误")
	assert.Equal(t, 9, pos.X, "边界X网格坐标应为9")
	assert.Equal(t, 9, pos.Y, "边界Y网格坐标应为9")

	// 测试超出边界坐标
	_, err = aoi.GetGridPosition(-10, -10)
	assert.Error(t, err, "获取超出边界坐标的网格位置应返回错误")

	_, err = aoi.GetGridPosition(1010, 1010)
	assert.Error(t, err, "获取超出边界坐标的网格位置应返回错误")
}
