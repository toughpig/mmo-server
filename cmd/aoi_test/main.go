package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"mmo-server/pkg/aoi"
)

// TestEntity 实现aoi.Entity接口的测试实体
type TestEntity struct {
	id       string
	x, y, z  float32
	typeID   int
	velocity struct {
		x, y, z float32
	}
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

// Move 移动实体
func (e *TestEntity) Move(deltaTime float32) {
	// 保存旧位置
	oldX, oldY, oldZ := e.x, e.y, e.z

	// 更新位置
	e.x += e.velocity.x * deltaTime
	e.y += e.velocity.y * deltaTime
	e.z += e.velocity.z * deltaTime

	// 简单的边界检查，当实体到达边界时反弹
	if e.x < 0 || e.x > 1000 {
		e.velocity.x = -e.velocity.x
		e.x = oldX + e.velocity.x*deltaTime
	}
	if e.y < 0 || e.y > 1000 {
		e.velocity.y = -e.velocity.y
		e.y = oldY + e.velocity.y*deltaTime
	}
	if e.z < 0 || e.z > 1000 {
		e.velocity.z = -e.velocity.z
		e.z = oldZ + e.velocity.z*deltaTime
	}
}

func main() {
	// 命令行参数
	playerCount := flag.Int("players", 500, "玩家数量")
	npcCount := flag.Int("npcs", 1000, "NPC数量")
	gridCount := flag.Int("grids", 20, "网格数量")
	iterations := flag.Int("iterations", 100, "模拟迭代次数")
	stepTime := flag.Float64("steptime", 0.1, "每次迭代的时间步长(秒)")
	queryRadius := flag.Float64("radius", 100.0, "实体查询半径")
	flag.Parse()

	fmt.Printf("AOI系统测试\n")
	fmt.Printf("玩家数量: %d, NPC数量: %d, 网格数量: %d x %d\n", *playerCount, *npcCount, *gridCount, *gridCount)
	fmt.Printf("迭代次数: %d, 时间步长: %.2f秒, 查询半径: %.2f\n", *iterations, *stepTime, *queryRadius)

	// 初始化随机数生成器
	rand.Seed(time.Now().UnixNano())

	// 创建AOI管理器
	aoiManager := aoi.NewAOIManager(0, 1000, 0, 1000, *gridCount)

	// 创建并添加玩家实体
	players := make([]*TestEntity, *playerCount)
	for i := 0; i < *playerCount; i++ {
		players[i] = &TestEntity{
			id:     fmt.Sprintf("player_%d", i),
			x:      rand.Float32() * 1000,
			y:      0,
			z:      rand.Float32() * 1000,
			typeID: 0, // 玩家类型
		}
		// 随机速度，[-10,10]范围
		players[i].velocity.x = (rand.Float32() * 20) - 10
		players[i].velocity.y = 0
		players[i].velocity.z = (rand.Float32() * 20) - 10

		err := aoiManager.AddEntity(players[i])
		if err != nil {
			log.Fatalf("添加玩家实体失败: %v", err)
		}
	}

	// 创建并添加NPC实体
	npcs := make([]*TestEntity, *npcCount)
	for i := 0; i < *npcCount; i++ {
		npcs[i] = &TestEntity{
			id:     fmt.Sprintf("npc_%d", i),
			x:      rand.Float32() * 1000,
			y:      0,
			z:      rand.Float32() * 1000,
			typeID: 1, // NPC类型
		}
		// 随机速度，[-5,5]范围
		npcs[i].velocity.x = (rand.Float32() * 10) - 5
		npcs[i].velocity.y = 0
		npcs[i].velocity.z = (rand.Float32() * 10) - 5

		err := aoiManager.AddEntity(npcs[i])
		if err != nil {
			log.Fatalf("添加NPC实体失败: %v", err)
		}
	}

	// 统计信息
	totalTime := 0.0
	totalQueries := 0
	totalEntitiesFound := 0
	totalUpdates := 0
	totalUpdateTime := 0.0

	// 开始模拟
	for iteration := 0; iteration < *iterations; iteration++ {
		fmt.Printf("\r迭代 %d/%d", iteration+1, *iterations)

		// 更新所有实体位置
		updateStartTime := time.Now()
		for i := 0; i < *playerCount; i++ {
			oldX, oldZ := players[i].x, players[i].z
			players[i].Move(float32(*stepTime))
			err := aoiManager.UpdateEntity(players[i], oldX, oldZ)
			if err != nil {
				log.Printf("更新玩家位置失败: %v", err)
				continue
			}
			totalUpdates++
		}

		for i := 0; i < *npcCount; i++ {
			oldX, oldZ := npcs[i].x, npcs[i].z
			npcs[i].Move(float32(*stepTime))
			err := aoiManager.UpdateEntity(npcs[i], oldX, oldZ)
			if err != nil {
				log.Printf("更新NPC位置失败: %v", err)
				continue
			}
			totalUpdates++
		}
		updateEndTime := time.Now()
		totalUpdateTime += updateEndTime.Sub(updateStartTime).Seconds()

		// 为所有玩家进行查询测试
		startTime := time.Now()
		for i := 0; i < *playerCount; i++ {
			// 每隔10个玩家进行一次查询
			if i%10 == 0 {
				entities, err := aoiManager.GetEntitiesByDistance(
					players[i].x, players[i].y, players[i].z, float32(*queryRadius))
				if err != nil {
					log.Printf("获取附近实体失败: %v", err)
					continue
				}
				totalQueries++
				totalEntitiesFound += len(entities)
			}
		}
		endTime := time.Now()
		totalTime += endTime.Sub(startTime).Seconds()
	}
	fmt.Println() // 换行

	// 输出统计信息
	fmt.Printf("\n统计信息:\n")
	fmt.Printf("总更新次数: %d, 平均每次更新时间: %.6f毫秒\n",
		totalUpdates, (totalUpdateTime*1000)/float64(totalUpdates))
	fmt.Printf("总查询次数: %d, 总查询时间: %.2f秒\n", totalQueries, totalTime)
	fmt.Printf("平均每次查询时间: %.6f毫秒\n", (totalTime*1000)/float64(totalQueries))
	fmt.Printf("平均每次查询返回实体数: %.2f\n", float64(totalEntitiesFound)/float64(totalQueries))

	// 输出网格统计
	fmt.Printf("\n网格统计:\n")
	playerGridCount := make(map[string]int)
	for i := 0; i < *playerCount; i++ {
		gridPos, err := aoiManager.GetGridPosition(players[i].x, players[i].z)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("%d,%d", gridPos.X, gridPos.Y)
		playerGridCount[key]++
	}

	occupiedGrids := len(playerGridCount)
	totalGrids := (*gridCount) * (*gridCount)
	fmt.Printf("总网格数: %d, 有玩家的网格数: %d (%.2f%%)\n",
		totalGrids, occupiedGrids, float64(occupiedGrids)*100/float64(totalGrids))

	// 密度统计
	var maxDensity, totalDensity int
	for _, count := range playerGridCount {
		if count > maxDensity {
			maxDensity = count
		}
		totalDensity += count
	}

	fmt.Printf("最大网格密度: %d玩家/网格\n", maxDensity)
	fmt.Printf("平均网格密度: %.2f玩家/网格\n", float64(totalDensity)/float64(occupiedGrids))
}
