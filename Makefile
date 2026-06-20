.PHONY: test-unit test-integration test-all clean build build-linux build-linux-arm64 build-linux-musl build-windows fmt docker

# 版本号（优先取 Git tag，否则默认 dev）
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS := -ldflags="-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT)"

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

# 构建（本地平台）
build:
	go build -v $(LDFLAGS) -o vortex .
	@echo "Build completed: $(VERSION)"

# 交叉编译 Linux amd64 (glibc)
build-linux:
	GOOS=linux GOARCH=amd64 go build -v $(LDFLAGS) -o vortex-linux-amd64 .
	@echo "Cross-compile linux/amd64 completed: $(VERSION)"

# 交叉编译 Linux arm64 (glibc)
build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -v $(LDFLAGS) -o vortex-linux-arm64 .
	@echo "Cross-compile linux/arm64 completed: $(VERSION)"

# 交叉编译 Linux amd64 (静态链接, musl)
build-linux-musl:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags="-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -extldflags=-static" -o vortex-linux-musl-amd64 .
	@echo "Cross-compile linux/amd64 (musl, static) completed: $(VERSION)"

# 交叉编译 Windows
build-windows:
	GOOS=windows GOARCH=amd64 go build -v $(LDFLAGS) -o vortex.exe .
	@echo "Cross-compile windows/amd64 completed: $(VERSION)"

# 构建 Docker 镜像
docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t vortex:$(VERSION) -f Dockerfile .
	docker tag vortex:$(VERSION) vortex:latest
	@echo "Docker build completed: $(VERSION)"

# 格式化代码
fmt:
	gofmt -w .
	@echo "Code formatted"

# 清理
clean:
	rm -f vortex vortex-linux-amd64 vortex-linux-arm64 vortex-linux-musl-amd64 vortex.exe
	rm -f unit-coverage.txt integration-coverage.txt
	@echo "Build artifacts cleaned"
