#!/usr/bin/env bash
# =============================================================================
# Vortex k6 统一压测脚本
#
# 用法:
#   ./run.sh                      # 冒烟测试
#   ./run.sh stress               # 压力测试
#   ./run.sh stress html          # 压力测试 + HTML报告
#   ./run.sh soak html,json       # 耐力测试 + 多格式报告
#   ./run.sh soak html 5m         # 耐力测试 + 报告 + 5分钟
#   ./run.sh spike html 0 http://192.168.1.100:9178  # 指定服务器
#   ./run.sh -h                   # 帮助
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

TEST=${1:-smoke}
OUT=${2:-console}
DURATION=${3:-}
BASE_URL=${4:-http://localhost:9178}

# ---- 帮助 ----
if [ "$TEST" = "-h" ] || [ "$TEST" = "--help" ]; then
    cat <<'EOF'
用法: ./run.sh [test] [output] [duration] [url]

  test     压测类型: smoke(默认), stress, spike, soak
  output   输出格式: console(默认), html, json, csv (可组合: html,json)
  duration 持续时间: 如 5m, 30s (默认使用脚本内置时长)
  url      目标地址: 如 http://192.168.1.100:9178

示例:
  ./run.sh                      冒烟测试
  ./run.sh stress               压力测试
  ./run.sh stress html          压力测试 + 生成HTML报告
  ./run.sh soak html,json       耐力测试 + HTML+JSON报告
  ./run.sh spike html 0 http://10.0.0.5:9178
EOF
    exit 0
fi

# ---- 校验 ----
case "$TEST" in
    smoke|stress|spike|soak) ;;
    *) echo "✖ 无效测试类型: $TEST (可选: smoke, stress, spike, soak)" >&2; exit 1 ;;
esac

if ! command -v k6 &>/dev/null; then
    echo "✖ 未找到 k6，请先安装: https://k6.io/docs/getting-started/installation/" >&2
    exit 1
fi

TEST_SCRIPT="${SCRIPT_DIR}/${TEST}.js"
if [ ! -f "$TEST_SCRIPT" ]; then
    echo "✖ 找不到测试脚本: $TEST_SCRIPT" >&2
    exit 1
fi

# ---- 构建 k6 参数 ----
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
HAS_OUTPUT=false

K6_ARGS=("run")

case ",$OUT," in
    *,html,*)
        mkdir -p "$SCRIPT_DIR/reports"
        K6_ARGS+=("--out" "html=${SCRIPT_DIR}/reports/vortex-${TEST}-${TIMESTAMP}.html")
        HAS_OUTPUT=true
        ;;&
    *,json,*)
        mkdir -p "$SCRIPT_DIR/reports"
        K6_ARGS+=("--out" "json=${SCRIPT_DIR}/reports/vortex-${TEST}-${TIMESTAMP}.json")
        HAS_OUTPUT=true
        ;;&
    *,csv,*)
        mkdir -p "$SCRIPT_DIR/reports"
        K6_ARGS+=("--out" "csv=${SCRIPT_DIR}/reports/vortex-${TEST}-${TIMESTAMP}.csv")
        HAS_OUTPUT=true
        ;;
esac

if [ -n "$DURATION" ]; then
    K6_ARGS+=("--duration" "$DURATION")
fi

export K6_BASE_URL="$BASE_URL"
K6_ARGS+=("$TEST_SCRIPT")

# ---- 打印摘要 ----
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Vortex k6 压测"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  测试     : $TEST"
echo "  目标     : $BASE_URL"
[ -n "$DURATION" ] && echo "  时长     : $DURATION"
[ "$HAS_OUTPUT" = true ] && echo "  报告     : reports/vortex-${TEST}-${TIMESTAMP}.*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# ---- 执行 ----
set +e
k6 "${K6_ARGS[@]}"
EXIT_CODE=$?
set -e

echo ""
if [ "$EXIT_CODE" -eq 0 ]; then
    echo "✔ 测试完成: $TEST"
else
    echo "⚠ 测试结束 (exit code: $EXIT_CODE)"
fi
exit "$EXIT_CODE"