#!/bin/bash

set -e

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

# Run specific component tests
echo "Running protocol tests..."
./bin/protocol_test

echo "All tests completed!" 