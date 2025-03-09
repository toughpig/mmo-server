#!/bin/bash

LOG_DIR="./logs"
PID_FILE="$LOG_DIR/metrics.pid"

# 检查PID文件是否存在
if [ ! -f "$PID_FILE" ]; then
    echo "未找到监控服务PID文件: $PID_FILE"
    
    # 尝试查找监控服务进程
    METRICS_PIDS=$(pgrep -f "bin/metrics")
    if [ -n "$METRICS_PIDS" ]; then
        echo "找到监控服务进程: $METRICS_PIDS"
        echo "正在停止进程..."
        
        # 尝试正常终止进程
        kill $METRICS_PIDS
        
        # 等待进程终止
        sleep 2
        
        # 检查进程是否仍在运行
        if pgrep -f "bin/metrics" > /dev/null; then
            echo "进程未能正常终止，正在强制终止..."
            pkill -9 -f "bin/metrics"
            sleep 1
        fi
        
        echo "监控服务已停止。"
    else
        echo "未找到正在运行的监控服务进程。"
    fi
    
    exit 0
fi

# 读取PID
METRICS_PID=$(cat "$PID_FILE")
if [ -z "$METRICS_PID" ]; then
    echo "PID文件为空: $PID_FILE"
    rm "$PID_FILE"
    exit 1
fi

# 检查进程是否存在
if ! ps -p $METRICS_PID > /dev/null; then
    echo "进程 (PID: $METRICS_PID) 不存在。"
    rm "$PID_FILE"
    exit 0
fi

# 停止进程
echo "正在停止监控服务 (PID: $METRICS_PID)..."
kill $METRICS_PID

# 等待进程终止
MAX_WAIT=10
for i in $(seq 1 $MAX_WAIT); do
    if ! ps -p $METRICS_PID > /dev/null; then
        echo "监控服务已停止。"
        rm "$PID_FILE"
        exit 0
    fi
    sleep 1
    echo "等待进程终止... ($i/$MAX_WAIT)"
done

# 如果进程仍在运行，强制终止
echo "进程未能在规定时间内终止，正在强制终止..."
kill -9 $METRICS_PID
sleep 1

if ! ps -p $METRICS_PID > /dev/null; then
    echo "监控服务已强制停止。"
else
    echo "无法停止监控服务。"
    exit 1
fi

# 删除PID文件
rm "$PID_FILE"
echo "已删除PID文件。" 