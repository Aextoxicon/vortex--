.PHONY: test-unit test-integration test-all clean build build-linux build-linux-musl fmt

# 运行所有单元测试（无需 Docker）
test-unit:
	@echo "Running unit tests (no Docker required)..."
	cargo test --lib -- --test-threads=1
	@echo "Unit tests completed"

# 运行所有集成测试（需要 Docker）
test-integration:
	@echo "Running integration tests (Docker required)..."
	cargo test --test '*' --verbose
	@echo "Integration tests completed"

# 运行所有测试
test-all: test-unit test-integration
	@echo "All tests completed"

# 构建（本地平台）
build:
	cargo build --release

# 交叉编译 Linux x86_64 (glibc)
build-linux:
	cargo zigbuild --release --target x86_64-unknown-linux-gnu

# 交叉编译 Linux x86_64 (musl, 静态链接)
build-linux-musl:
	cargo zigbuild --release --target x86_64-unknown-linux-musl

# 格式化代码
fmt:
	cargo fmt
	@echo "Code formatted"

# 清理
clean:
	cargo clean
	@echo "Build artifacts cleaned"

# =============================================================================
# k6 压测（需要 Docker Compose，Linux/macOS 推荐）
# =============================================================================
# 注：Windows 用户请直接使用:
#   cd loadtest && .\run.bat [test] [output] [duration] [url]
# 例:
#   cd loadtest && .\run.bat stress html
# =============================================================================

# 前置条件：已运行 docker compose up -d
COMPOSE_FILES := -f docker-compose.yml -f docker-compose.test.yml

.PHONY: test-loadtest test-loadtest-stress test-loadtest-spike test-loadtest-soak

# 冒烟测试（默认，1 VU, 30s）
test-loadtest:
	@echo "Running k6 smoke test..."
	docker compose $(COMPOSE_FILES) run --rm k6
	@echo "k6 smoke test completed"

# 压力测试（阶梯加压，找系统瓶颈）
test-loadtest-stress:
	@echo "Running k6 stress test..."
	docker compose $(COMPOSE_FILES) run --rm k6 run /loadtest/stress.js
	@echo "k6 stress test completed"

# 尖峰测试（突发高并发）
test-loadtest-spike:
	@echo "Running k6 spike test..."
	docker compose $(COMPOSE_FILES) run --rm k6 run /loadtest/spike.js
	@echo "k6 spike test completed"

# 耐力测试（长时间运行，检查稳定性）
test-loadtest-soak:
	@echo "Running k6 soak test..."
	docker compose $(COMPOSE_FILES) run --rm k6 run /loadtest/soak.js
	@echo "k6 soak test completed"
