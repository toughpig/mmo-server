#!/bin/bash

# 配置参数
METRICS_PORT=9100
METRICS_INTERVAL=10s
BIN_DIR="./bin"
LOG_DIR="./logs"
METRICS_BIN="metrics"

# 确保二进制文件已构建
if [ ! -f "$BIN_DIR/$METRICS_BIN" ]; then
    echo "未找到监控服务二进制文件: $BIN_DIR/$METRICS_BIN"
    echo "正在构建..."
    
    # 检查二进制目录是否存在
    if [ ! -d "$BIN_DIR" ]; then
        mkdir -p "$BIN_DIR"
    fi
    
    # 构建二进制文件
    go build -o "$BIN_DIR/$METRICS_BIN" ./cmd/metrics
    
    if [ $? -ne 0 ]; then
        echo "构建失败，请检查错误信息。"
        exit 1
    fi
    
    echo "构建成功。"
fi

# 创建日志目录
if [ ! -d "$LOG_DIR" ]; then
    mkdir -p "$LOG_DIR"
fi

# 检查是否已经有监控服务在运行
if [ -f "$LOG_DIR/metrics.pid" ]; then
    PID=$(cat "$LOG_DIR/metrics.pid")
    if ps -p $PID > /dev/null; then
        echo "监控服务已经在运行 (PID: $PID)"
        echo "如果要重启服务，请先运行 stop_metrics.sh 脚本。"
        exit 1
    else
        echo "发现过期的PID文件，将删除。"
        rm "$LOG_DIR/metrics.pid"
    fi
fi

# 启动监控服务
echo "启动监控服务..."
nohup "$BIN_DIR/$METRICS_BIN" -addr=":$METRICS_PORT" -interval="$METRICS_INTERVAL" > "$LOG_DIR/metrics.log" 2>&1 &

# 保存进程ID
METRICS_PID=$!
echo $METRICS_PID > "$LOG_DIR/metrics.pid"
echo "监控服务已启动 (PID: $METRICS_PID)"
echo "监听端口: $METRICS_PORT"
echo "收集间隔: $METRICS_INTERVAL"
echo "日志文件: $LOG_DIR/metrics.log"

# 检查服务是否成功启动
sleep 2
if ps -p $METRICS_PID > /dev/null; then
    echo "监控服务运行正常。"
    echo "通过以下网址访问指标: http://localhost:$METRICS_PORT/metrics"
else
    echo "监控服务启动失败，请检查日志文件: $LOG_DIR/metrics.log"
    exit 1
fi 