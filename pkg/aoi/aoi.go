package aoi

import (
	"fmt"
	"math"
	"sync"
)

// Entity 表示一个可以放入AOI系统的实体
type Entity interface {
	GetID() string                            // 获取实体唯一ID
	GetPosition() (float32, float32, float32) // 获取实体当前位置(x,y,z)
	GetTypeID() int                           // 获取实体类型ID
}

// GridPos 表示二维网格坐标
type GridPos struct {
	X int
	Y int
}

// AOIManager 基于网格的兴趣区域管理器
type AOIManager struct {
	minX      float32      // 场景X最小坐标
	maxX      float32      // 场景X最大坐标
	minZ      float32      // 场景Z最小坐标
	maxZ      float32      // 场景Z最大坐标
	gridWidth float32      // 网格宽度
	gridCount int          // X轴和Z轴的网格数量
	grids     [][]*Grid    // 网格二维数组
	mutex     sync.RWMutex // 保护并发访问
}

// Grid 表示一个网格
type Grid struct {
	gridPos     GridPos           // 网格坐标
	entities    map[string]Entity // 网格中的实体
	playerCount int               // 网格中的玩家数量
	mutex       sync.RWMutex      // 保护并发访问
}

// NewAOIManager 创建一个新的AOI管理器
func NewAOIManager(minX, maxX, minZ, maxZ float32, gridCount int) *AOIManager {
	gridWidth := (maxX - minX) / float32(gridCount)

	// 创建AOI管理器
	aoi := &AOIManager{
		minX:      minX,
		maxX:      maxX,
		minZ:      minZ,
		maxZ:      maxZ,
		gridWidth: gridWidth,
		gridCount: gridCount,
		grids:     make([][]*Grid, gridCount),
	}

	// 初始化网格
	for i := 0; i < gridCount; i++ {
		aoi.grids[i] = make([]*Grid, gridCount)
		for j := 0; j < gridCount; j++ {
			aoi.grids[i][j] = &Grid{
				gridPos:  GridPos{X: i, Y: j},
				entities: make(map[string]Entity),
			}
		}
	}

	return aoi
}

// GetGrid 获取指定坐标所在的网格
func (m *AOIManager) GetGrid(x, z float32) (*Grid, error) {
	// 检查坐标是否在场景范围内
	if x < m.minX || x >= m.maxX || z < m.minZ || z >= m.maxZ {
		return nil, fmt.Errorf("坐标 (%f, %f) 超出场景范围 [%f, %f, %f, %f]",
			x, z, m.minX, m.maxX, m.minZ, m.maxZ)
	}

	// 计算网格索引
	gridX := int((x - m.minX) / m.gridWidth)
	gridZ := int((z - m.minZ) / m.gridWidth)

	// 边界检查
	if gridX >= m.gridCount {
		gridX = m.gridCount - 1
	}
	if gridZ >= m.gridCount {
		gridZ = m.gridCount - 1
	}

	return m.grids[gridX][gridZ], nil
}

// GetSurroundingGridsByRange 获取以某个网格为中心，指定范围内的所有网格
func (m *AOIManager) GetSurroundingGridsByRange(centerGrid *Grid, range_ int) []*Grid {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var surroundGrids []*Grid
	centerX := centerGrid.gridPos.X
	centerY := centerGrid.gridPos.Y

	// 获取周围的网格
	for x := centerX - range_; x <= centerX+range_; x++ {
		for y := centerY - range_; y <= centerY+range_; y++ {
			if x >= 0 && x < m.gridCount && y >= 0 && y < m.gridCount {
				surroundGrids = append(surroundGrids, m.grids[x][y])
			}
		}
	}

	return surroundGrids
}

// GetSurroundingGrids 获取指定位置周围的网格
func (m *AOIManager) GetSurroundingGrids(x, z float32, range_ int) ([]*Grid, error) {
	// 获取中心网格
	centerGrid, err := m.GetGrid(x, z)
	if err != nil {
		return nil, err
	}

	return m.GetSurroundingGridsByRange(centerGrid, range_), nil
}

// GetSurroundingEntities 获取指定位置周围的实体
func (m *AOIManager) GetSurroundingEntities(x, z float32, range_ int) ([]Entity, error) {
	// 获取周围网格
	grids, err := m.GetSurroundingGrids(x, z, range_)
	if err != nil {
		return nil, err
	}

	// 收集所有实体
	entityMap := make(map[string]Entity)
	for _, grid := range grids {
		grid.mutex.RLock()
		for _, entity := range grid.entities {
			entityMap[entity.GetID()] = entity
		}
		grid.mutex.RUnlock()
	}

	// 转换为数组
	entities := make([]Entity, 0, len(entityMap))
	for _, entity := range entityMap {
		entities = append(entities, entity)
	}

	return entities, nil
}

// GetEntitiesByDistance 获取指定位置周围指定距离内的实体
func (m *AOIManager) GetEntitiesByDistance(x, y, z float32, distance float32) ([]Entity, error) {
	// 获取周围网格，计算网格范围
	gridRange := int(math.Ceil(float64(distance / m.gridWidth)))
	surroundingEntities, err := m.GetSurroundingEntities(x, z, gridRange)
	if err != nil {
		return nil, err
	}

	// 根据距离过滤实体
	var entities []Entity
	distanceSquared := distance * distance

	for _, entity := range surroundingEntities {
		ex, ey, ez := entity.GetPosition()
		// 计算距离的平方，避免开平方操作
		dx := ex - x
		dy := ey - y
		dz := ez - z
		if (dx*dx + dy*dy + dz*dz) <= float32(distanceSquared) {
			entities = append(entities, entity)
		}
	}

	return entities, nil
}

// AddEntity 将实体添加到AOI系统中
func (m *AOIManager) AddEntity(entity Entity) error {
	x, _, z := entity.GetPosition()

	// 获取实体所在网格
	grid, err := m.GetGrid(x, z)
	if err != nil {
		return err
	}

	// 添加到网格
	grid.mutex.Lock()
	defer grid.mutex.Unlock()

	grid.entities[entity.GetID()] = entity
	if entity.GetTypeID() == 0 { // 假设0是玩家类型
		grid.playerCount++
	}

	return nil
}

// RemoveEntity 从AOI系统中移除实体
func (m *AOIManager) RemoveEntity(entity Entity) error {
	x, _, z := entity.GetPosition()

	// 获取实体所在网格
	grid, err := m.GetGrid(x, z)
	if err != nil {
		return err
	}

	// 从网格中移除
	grid.mutex.Lock()
	defer grid.mutex.Unlock()

	id := entity.GetID()
	if _, exists := grid.entities[id]; exists {
		delete(grid.entities, id)
		if entity.GetTypeID() == 0 { // 假设0是玩家类型
			grid.playerCount--
		}
	}

	return nil
}

// UpdateEntity 更新实体在AOI系统中的位置
func (m *AOIManager) UpdateEntity(entity Entity, oldX, oldZ float32) error {
	// 获取新位置
	newX, _, newZ := entity.GetPosition()

	// 获取旧网格和新网格
	oldGrid, err := m.GetGrid(oldX, oldZ)
	if err != nil {
		return err
	}

	newGrid, err := m.GetGrid(newX, newZ)
	if err != nil {
		return err
	}

	// 如果网格没变，不需要更新
	if oldGrid.gridPos.X == newGrid.gridPos.X && oldGrid.gridPos.Y == newGrid.gridPos.Y {
		return nil
	}

	// 从旧网格中移除
	oldGrid.mutex.Lock()
	id := entity.GetID()
	if _, exists := oldGrid.entities[id]; exists {
		delete(oldGrid.entities, id)
		if entity.GetTypeID() == 0 { // 假设0是玩家类型
			oldGrid.playerCount--
		}
	}
	oldGrid.mutex.Unlock()

	// 添加到新网格
	newGrid.mutex.Lock()
	newGrid.entities[id] = entity
	if entity.GetTypeID() == 0 { // 假设0是玩家类型
		newGrid.playerCount++
	}
	newGrid.mutex.Unlock()

	return nil
}

// GetNearbyPlayers 获取指定坐标附近的玩家数量
func (m *AOIManager) GetNearbyPlayers(x, z float32, range_ int) (int, error) {
	// 获取周围网格
	grids, err := m.GetSurroundingGrids(x, z, range_)
	if err != nil {
		return 0, err
	}

	// 计算总玩家数
	playerCount := 0
	for _, grid := range grids {
		grid.mutex.RLock()
		playerCount += grid.playerCount
		grid.mutex.RUnlock()
	}

	return playerCount, nil
}

// GetGridSize 获取AOI网格数量信息
func (m *AOIManager) GetGridSize() (int, int) {
	return m.gridCount, m.gridCount
}

// GetGridPosition 计算实体所在的网格位置
func (m *AOIManager) GetGridPosition(x, z float32) (GridPos, error) {
	// 检查坐标是否在场景范围内
	if x < m.minX || x >= m.maxX || z < m.minZ || z >= m.maxZ {
		return GridPos{}, fmt.Errorf("坐标 (%f, %f) 超出场景范围 [%f, %f, %f, %f]",
			x, z, m.minX, m.maxX, m.minZ, m.maxZ)
	}

	// 计算网格索引
	gridX := int((x - m.minX) / m.gridWidth)
	gridZ := int((z - m.minZ) / m.gridWidth)

	// 边界检查
	if gridX >= m.gridCount {
		gridX = m.gridCount - 1
	}
	if gridZ >= m.gridCount {
		gridZ = m.gridCount - 1
	}

	return GridPos{X: gridX, Y: gridZ}, nil
}
