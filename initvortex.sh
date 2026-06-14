#!/bin/bash
set -e

echo "=== Vortex 全服务部署脚本 ==="

# ==================== 前置检查 ====================
if ! command -v docker &>/dev/null; then
    echo "错误: 未检测到 Docker，请先安装 Docker"
    exit 1
fi
if ! docker compose version &>/dev/null; then
    echo "错误: Docker Compose 不可用，请升级 Docker"
    exit 1
fi

# ==================== 环境变量文件 ====================
if [ -f ".env" ]; then
    echo "检测到已有 .env 文件，将复用现有配置"
else
    echo "生成 .env 配置文件..."

    # JWT 密钥
    JWT_SECRET=$(openssl rand -base64 32)

    # 数据库密码（可选覆盖）
    DB_PASSWORD=${POSTGRES_PASSWORD:-$(openssl rand -base64 24)}

    cat > .env <<EOF
# Vortex 环境配置 — 由 initvortex.sh 自动生成
# 复制此文件可复用到其他部署

# JWT 密钥（请妥善保管）
JWT_SECRET=$JWT_SECRET

# 数据库密码（如需自定义请在运行脚本前设置 POSTGRES_PASSWORD 环境变量）
POSTGRES_PASSWORD=$DB_PASSWORD
EOF

    echo "  JWT_SECRET 已生成"
    echo "  数据库密码已保存至 .env 文件"
    echo "  ⚠ 请妥善保管这些凭据！"
fi

# ==================== 数据目录 ====================
# 如果使用命名卷则由 Docker 自动管理，无需手动创建

# ==================== 启动服务 ====================
echo ""
echo "正在拉取镜像并启动所有服务..."
docker compose up -d

echo ""
echo "=== 部署完成 ==="
echo "服务端口: 9178"
echo ""
echo "常用命令:"
echo "  查看状态:   docker compose ps"
echo "  查看日志:   docker compose logs -f"
echo "  停止服务:   docker compose down"
echo "  完全清理:   docker compose down -v"
echo ""
