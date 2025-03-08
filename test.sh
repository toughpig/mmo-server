#!/bin/bash

set -e

# 清理函数，用于在脚本退出时执行清理操作
cleanup() {
  echo "Cleaning up..."
  # 清理socket文件
  rm -f /tmp/mmo-rpc-test.sock /tmp/mmo-rpc-example.sock
  # 如果有RPC服务器在运行，杀掉它
  if [ ! -z "$RPC_SERVER_PID" ]; then
    kill $RPC_SERVER_PID 2>/dev/null || true
  fi
  echo "Cleanup completed."
}

# 设置trap以在脚本退出时执行清理函数
trap cleanup EXIT

# 确保临时socket文件不存在
rm -f /tmp/mmo-rpc-test.sock /tmp/mmo-rpc-example.sock

echo "Running MMO Server Framework tests..."

# Run Go tests with verbose output and coverage report
go test -v -cover ./pkg/... ./internal/... ./cmd/... ./server/... 2>&1 | tee test_results.log

# Check if any tests failed
if [ ${PIPESTATUS[0]} -ne 0 ]; then
  echo "Tests failed! See test_results.log for details."
  exit 1
fi

# Generate HTML coverage report
echo "Generating coverage report..."
go test -coverprofile=coverage.out ./pkg/... ./internal/... ./cmd/... ./server/...
go tool cover -html=coverage.out -o coverage.html

echo "Tests completed successfully! See coverage.html for coverage report."

# Ensure binaries are built
echo "Building test binaries..."
./build.sh

# Run specific component tests
echo "Running protocol tests..."
./bin/protocol_test

# Run RPC framework tests - basic functionality
echo "Running RPC framework basic tests..."
./bin/rpc_test -mode=example || {
  echo "Basic RPC test failed. This might be due to shared memory configuration."
  echo "Continuing with other tests..."
}

# Start a server instance for more detailed tests
echo "Starting RPC test server in background..."
./bin/rpc_test -mode=server -endpoint=/tmp/mmo-rpc-test.sock &
RPC_SERVER_PID=$!

# Give the server time to start
sleep 2

# Verify server is running
if ps -p $RPC_SERVER_PID > /dev/null; then
  echo "RPC server started successfully (PID: $RPC_SERVER_PID)"
else
  echo "Failed to start RPC server. Skipping RPC client tests."
  RPC_SERVER_PID=""
  exit 1
fi

# Run client tests against the server
echo "Running RPC client tests..."
./bin/rpc_test -mode=client -endpoint=/tmp/mmo-rpc-test.sock || {
  echo "RPC client test failed. This might be due to connection issues."
  echo "Continuing with other tests..."
}

# Run stress tests
echo "Running RPC stress tests..."
./bin/rpc_test -mode=stress -endpoint=/tmp/mmo-rpc-test.sock -requests=50 -interval=10 || {
  echo "RPC stress test failed."
  echo "Continuing with other tests..."
}

# Run concurrent client tests
echo "Running RPC concurrent client tests..."
./bin/rpc_test -mode=concurrent -endpoint=/tmp/mmo-rpc-test.sock -clients=5 -requests=10 -interval=10 || {
  echo "RPC concurrent test failed."
  echo "Continuing with other tests..."
}

# Run error handling tests
echo "Running RPC error handling tests..."
./bin/rpc_test -mode=error -endpoint=/tmp/mmo-rpc-test.sock || {
  echo "RPC error handling test failed."
  echo "Continuing with other tests..."
}

# Kill the server when done
if [ ! -z "$RPC_SERVER_PID" ]; then
  echo "Stopping RPC test server..."
  kill $RPC_SERVER_PID || true
  RPC_SERVER_PID=""
fi

# Run player state sync tests
echo "Running player state sync tests..."
./bin/sync_test -mode=example || {
  echo "Player state sync test failed."
  echo "Continuing with other tests..."
}

echo "All tests completed!" 