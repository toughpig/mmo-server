#!/bin/bash

# 检查PID文件是否存在
PID_FILE="logs/envoy.pid"
if [ ! -f "$PID_FILE" ]; then
    echo "Envoy PID文件不存在: $PID_FILE"
    echo "Envoy可能未运行或未通过start_envoy.sh脚本启动。"
    
    # 尝试查找所有Envoy进程并终止
    ENVOY_PIDS=$(pgrep envoy)
    if [ -n "$ENVOY_PIDS" ]; then
        echo "找到Envoy进程: $ENVOY_PIDS"
        echo "正在停止所有Envoy进程..."
        kill $ENVOY_PIDS
        sleep 2
        
        # 检查是否还有进程运行
        if pgrep envoy > /dev/null; then
            echo "强制终止所有Envoy进程..."
            pkill -9 envoy
        fi
        
        echo "所有Envoy进程已停止。"
    else
        echo "未找到正在运行的Envoy进程。"
    fi
    
    exit 0
fi

# 读取PID
ENVOY_PID=$(cat "$PID_FILE")
if [ -z "$ENVOY_PID" ]; then
    echo "PID文件为空: $PID_FILE"
    rm -f "$PID_FILE"
    exit 1
fi

# 检查进程是否存在
if ! ps -p $ENVOY_PID > /dev/null; then
    echo "Envoy进程 (PID: $ENVOY_PID) 不存在。"
    rm -f "$PID_FILE"
    exit 1
fi

# 停止Envoy
echo "正在停止Envoy (PID: $ENVOY_PID)..."
kill $ENVOY_PID

# 等待进程终止
MAX_WAIT=10
for i in $(seq 1 $MAX_WAIT); do
    if ! ps -p $ENVOY_PID > /dev/null; then
        echo "Envoy已停止。"
        rm -f "$PID_FILE"
        exit 0
    fi
    sleep 1
done

# 如果进程仍在运行，强制终止
echo "Envoy未能正常停止，正在强制终止..."
kill -9 $ENVOY_PID
sleep 1

if ! ps -p $ENVOY_PID > /dev/null; then
    echo "Envoy已强制停止。"
else
    echo "无法停止Envoy进程。"
    exit 1
fi

# 清理PID文件
rm -f "$PID_FILE" 