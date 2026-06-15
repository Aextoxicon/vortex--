# Vortex 压测脚本 (k6)

## 目录结构

```
loadtest/
├── run.bat              # [推荐] 统一压测脚本 (Windows CMD 可直接运行)
├── run.ps1              # 统一压测脚本 (Windows PowerShell)
├── run.sh               # 统一压测脚本 (macOS/Linux)
├── config.js            # 全局配置（BASE_URL、阈值等）
├── lib/
│   ├── setup.js         # 批量注册/登录用户 + Token 管理
│   └── helpers.js       # 随机数据生成等工具函数
├── smoke.js             # 冒烟测试（小并发，验证功能正常）
├── stress.js            # 压力测试（逐步加压，找瓶颈）
├── spike.js             # 尖峰测试（突发高并发）
├── soak.js              # 耐力测试（长时间运行，检查稳定性）
└── README.md
```

## 安装 k6

- **Windows**: `winget install k6` 或下载 exe
- **macOS**: `brew install k6`
- **Linux**: 从 [k6.io](https://k6.io/docs/getting-started/installation/) 下载

## 快速使用（推荐）

统一脚本支持位置参数，按顺序传入即可。支持自动生成带时间戳的报告。

### Windows (CMD / PowerShell)

```powershell
cd loadtest

.\run.bat                        # 冒烟测试（默认）
.\run.bat stress                 # 压力测试
.\run.bat stress html            # 压力测试 + 生成 HTML 报告
.\run.bat soak html,json         # 耐力测试 + HTML + JSON 报告
.\run.bat soak html 5m           # 耐力测试 5 分钟 + HTML
.\run.bat spike csv              # 尖峰测试 + CSV 报告
.\run.bat spike html 0 http://192.168.1.100:9178   # 指定服务器
.\run.bat ?                      # 查看帮助
```

### macOS / Linux

```bash
cd loadtest
chmod +x run.sh

./run.sh                          # 冒烟测试（默认）
./run.sh stress                   # 压力测试
./run.sh stress html              # 压力测试 + HTML 报告
./run.sh soak html,json           # 耐力测试 + HTML + JSON 报告
./run.sh soak html 5m             # 耐力测试 5 分钟 + HTML
./run.sh spike csv                # 尖峰测试 + CSV 报告
./run.sh spike html 0 http://192.168.1.100:9178  # 指定服务器
./run.sh -h                       # 查看帮助
```

### 手动运行（不通过统一脚本）

```bash
cd loadtest

# 冒烟测试
k6 run smoke.js

# 压力测试 + HTML 报告
k6 run --out html=report.html stress.js

# 指定服务器（通过环境变量）
K6_BASE_URL=http://192.168.1.100:9178 k6 run smoke.js
```

### 输出报告

第2个参数指定输出格式，报告自动保存到 `loadtest/reports/` 目录，文件名格式为 `vortex-{test}-{timestamp}.{ext}`。

| 格式 | 说明 |
|------|------|
| console | 终端输出（默认） |
| json | 结构化数据，可用于后续分析 |
| html | 可视化 HTML 报告 |
| csv | 表格数据，适合导入 Excel/Sheet |

## 场景说明

| 脚本 | 并发模式 | 时长 | 目的 |
|------|----------|------|------|
| smoke.js | 固定 1 VU | 30s | 验证所有 API 基本正常 |
| stress.js | 阶梯 1→200→0 | ~3min | 逐步加压，找出系统瓶颈 |
| spike.js | 瞬间 0→300→0 | ~1min | 测试突发流量处理能力 |
| soak.js | 固定 50 VUs | 30min | 长时间运行，检查内存泄漏/稳定性 |

> smoke 和 soak 支持通过 `-VUs`/`-v` 和 `-Duration`/`-d` 自定义并发数和时长。
> stress 和 spike 使用内部 stages 阶梯控制，VUs/Duration 参数不生效。


## Docker Compose 测试（无需安装 k6）

如果你不想在宿主机安装 k6，或希望测试环境完全容器化，可以使用项目根目录的 `docker-compose.test.yml` 覆盖文件。

### 前置条件

```bash
# 确保已启动 Vortex 应用
cd /path/to/vortex

# 确保 .env 中 JWT_SECRET 已配置
# （参考 initvortex.ps1 / initvortex.sh 生成密钥）
docker compose up -d
```

### 使用方式

所有 k6 测试脚本都在容器内运行，无需本地安装 k6。

```bash
# 冒烟测试（默认，1 VU, 30s）
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm k6

# 压力测试（阶梯加压）
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm k6 run /loadtest/stress.js

# 尖峰测试（突发高并发）
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm k6 run /loadtest/spike.js

# 耐力测试（长时间运行）
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm k6 run /loadtest/soak.js
```

### 通过 Makefile 快捷运行

```bash
make test-loadtest             # 冒烟测试（默认）
make test-loadtest-stress      # 压力测试
make test-loadtest-spike       # 尖峰测试
make test-loadtest-soak        # 耐力测试
```

### 生成报告

```bash
# HTML 报告
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm
  -e K6_OUT=html=/loadtest/reports/report.html k6 run /loadtest/stress.js

# JSON 报告（便于后续分析）
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm
  -e K6_OUT=json=/loadtest/reports/report.json k6 run /loadtest/stress.js
```

报告文件将自动保存到 `loadtest/reports/` 目录。

### 指定目标服务器

默认测试目标为 Docker 内部 Vortex 服务（http://vortex:9178）。如需测试其他地址：

```bash
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm
  -e K6_BASE_URL=http://192.168.1.100:9178 k6 run /loadtest/smoke.js
```

### 交互式调试

```bash
docker compose -f docker-compose.yml -f docker-compose.test.yml run --rm --entrypoint sh k6
```

