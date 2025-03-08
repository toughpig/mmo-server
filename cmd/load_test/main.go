package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	connCount int32
	msgCount  int32
	errCount  int32
)

func main() {
	// 命令行参数
	addr := flag.String("addr", "localhost:8081", "服务器地址")
	path := flag.String("path", "/ws", "WebSocket路径")
	connections := flag.Int("conn", 100, "并发连接数")
	messagesPerConn := flag.Int("msg", 10, "每个连接发送的消息数量")
	interval := flag.Int("interval", 100, "消息间隔(毫秒)")
	rampUpTime := flag.Int("rampup", 5, "建立所有连接的时间(秒)")
	flag.Parse()

	log.Printf("并发测试 - 目标: %d 连接, 每个连接 %d 消息", *connections, *messagesPerConn)

	// 计算连接间隔
	connInterval := time.Duration(*rampUpTime) * time.Second / time.Duration(*connections)

	// 启动性能指标报告
	done := make(chan bool)
	go reportStats(done)

	// 创建等待组用于同步所有连接
	var wg sync.WaitGroup
	wg.Add(*connections)

	// 建立连接并发送消息
	for i := 0; i < *connections; i++ {
		go func(clientID int) {
			defer wg.Done()

			url := fmt.Sprintf("ws://%s%s", *addr, *path)
			c, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				log.Printf("[客户端 %d] 连接失败: %v", clientID, err)
				atomic.AddInt32(&errCount, 1)
				return
			}
			defer c.Close()

			// 连接成功，增加计数
			atomic.AddInt32(&connCount, 1)

			// 接收消息的goroutine
			msgDone := make(chan bool)
			go func() {
				defer close(msgDone)
				for {
					_, _, err := c.ReadMessage()
					if err != nil {
						// 连接关闭或错误
						return
					}

					atomic.AddInt32(&msgCount, 1)
				}
			}()

			// 发送消息
			for j := 0; j < *messagesPerConn; j++ {
				message := fmt.Sprintf("来自客户端 #%d 的测试消息 #%d", clientID, j+1)
				err := c.WriteMessage(websocket.TextMessage, []byte(message))
				if err != nil {
					log.Printf("[客户端 %d] 发送错误: %v", clientID, err)
					atomic.AddInt32(&errCount, 1)
					break
				}

				time.Sleep(time.Duration(*interval) * time.Millisecond)
			}

			// 等待所有响应接收完成
			time.Sleep(time.Second)

			// 关闭连接
			c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			<-msgDone

			// 连接已关闭，减少计数
			atomic.AddInt32(&connCount, -1)
		}(i)

		// 连接间隔
		time.Sleep(connInterval)
	}

	// 等待所有连接完成
	wg.Wait()

	// 停止性能指标报告
	done <- true
	<-done

	// 打印最终结果
	log.Printf("测试完成 - 总消息数: %d, 错误数: %d", atomic.LoadInt32(&msgCount), atomic.LoadInt32(&errCount))
	os.Exit(0)
}

// reportStats 定期报告性能指标
func reportStats(done chan bool) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	var lastMsgCount int32

	for {
		select {
		case <-done:
			done <- true
			return
		case <-ticker.C:
			currentMsgCount := atomic.LoadInt32(&msgCount)
			msgPerSec := currentMsgCount - lastMsgCount
			lastMsgCount = currentMsgCount
			elapsedSec := time.Since(startTime).Seconds()

			log.Printf("性能报告 - 连接数: %d, 消息数: %d (%.2f/秒), 已运行: %.1f秒, 错误数: %d",
				atomic.LoadInt32(&connCount),
				currentMsgCount,
				float64(msgPerSec),
				elapsedSec,
				atomic.LoadInt32(&errCount))
		}
	}
}
