#!/bin/bash

# 检查Envoy是否已安装
if ! command -v envoy &> /dev/null; then
    echo "Envoy未安装，请先安装Envoy。"
    echo "Ubuntu: sudo apt-get update && sudo apt-get install -y apt-transport-https ca-certificates curl gnupg-agent software-properties-common"
    echo "        curl -sL 'https://deb.dl.getenvoy.io/public/gpg.8115BA8E629CC074.key' | sudo gpg --dearmor -o /usr/share/keyrings/getenvoy-keyring.gpg"
    echo "        echo \"deb [arch=amd64 signed-by=/usr/share/keyrings/getenvoy-keyring.gpg] https://deb.dl.getenvoy.io/public/deb/ubuntu $(lsb_release -cs) main\" | sudo tee /etc/apt/sources.list.d/getenvoy.list"
    echo "        sudo apt-get update && sudo apt-get install -y getenvoy-envoy"
    exit 1
fi

# 检查配置文件是否存在
CONFIG_FILE="config/envoy/envoy.yaml"
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Envoy配置文件不存在: $CONFIG_FILE"
    exit 1
fi

# 创建日志目录
mkdir -p logs/envoy

# 启动Envoy
echo "启动Envoy代理..."
envoy -c "$CONFIG_FILE" --log-level info --log-path logs/envoy/envoy.log &

# 存储进程ID
ENVOY_PID=$!
echo $ENVOY_PID > logs/envoy.pid
echo "Envoy已启动 (PID: $ENVOY_PID)"
echo "配置文件: $CONFIG_FILE"
echo "日志文件: logs/envoy/envoy.log"

# 等待确认Envoy已启动
sleep 2
if ps -p $ENVOY_PID > /dev/null; then
    echo "Envoy运行正常。"
else
    echo "启动Envoy失败，请检查日志。"
    exit 1
fi 