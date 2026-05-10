.PHONY: test-unit test-integration test-all clean build fmt

# 运行所有单元测试（无需 Docker）
test-unit:
	@echo "Running unit tests (no Docker required)..."
	go test -v -race -coverprofile=unit-coverage.txt -covermode=atomic \
		-run "TestValidate|TestRateLimiter|TestMessageTableName|TestCalculate" ./...
	@echo "Unit tests completed"

# 运行所有集成测试（需要 Docker）
test-integration:
	@echo "Running integration tests (Docker required)..."
	go test -v -race -coverprofile=integration-coverage.txt -covermode=atomic ./...
	@echo "Integration tests completed"

# 运行所有测试
test-all: test-unit test-integration
	@echo "All tests completed"

# 构建
build:
	go build -v ./...

# 格式化代码
fmt:
	gofmt -w .
	@echo "Code formatted"

# 清理
clean:
	rm -f vortex unit-coverage.txt integration-coverage.txt
