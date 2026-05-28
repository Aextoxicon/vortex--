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
