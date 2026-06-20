# Vortex

关于collision的额外多人协作支持
但是不至于现在就投入使用
注：代码包含ai辅助，但是经过测试和人类审核，虽然说我可能会在一些地方写上极具个人风格的注释，但是这些东西会日后迁移到单独的文件里面，到时候只会留下有用的东西

# 项目结构

```
vortex/
├── main.go              # 应用入口：启动服务、初始化、路由注册
├── config.go            # 配置管理：环境变量加载与验证
├── error.go             # 错误处理：自定义错误类型和响应格式
├── account.go           # 用户模块：注册/登录/用户信息管理
├── messaging.go         # 消息模块：发送/接收/撤回消息、会话列表
├── friend.go            # 好友模块：好友请求/接受/拒绝/列表
├── groups.go            # 群组模块：创建群组/邀请/群组管理
├── jwt.go               # 认证模块：JWT Token 生成/验证/黑名单
├── ratelimit.go         # 限流模块：请求频率控制、防滥用
├── idgen.go             # ID 生成：分布式唯一 ID 生成器
├── migration.go         # 数据库迁移：表结构创建和版本管理
├── store.go             # 数据访问层：所有数据库 CRUD 操作
├── s3.go                # 文件模块：S3 预签名 URL 生成、文件权限控制
├── shared.go            # 共享组件：Service结构体、健康检查、通知系统
├── worker.go            # 后台任务：定时任务、数据清理、表分区管理
├── metrics.go           # 指标模块：性能监控和统计
└── test/                # 测试工具
    └── testutil/
        └── postgres.go  # 测试数据库连接工具
```

# 快速开始

# 环境要求

- Go 1.26+
- PostgreSQL 16+
- Docker (可选，用于测试)
- S3 服务

# 安装

```bash
git clone https://github.com/vortex--/vortex.git
cd vortex
go mod tidy
go build -o vortex
mkdir -p /opt/vortex/
mv vortex /opt/vortex/
# 然后按需配置其他组件，运行
```

# 配置

```bash
cp .env.example .env
# 编辑 .env 文件，设置必要的环境变量
```

生成 JWT 密钥：

```bash
openssl rand -base64 32
```

# Docker Compose部署（推荐）

```bash
# 1. 先设置 JWT 密钥
echo JWT_SECRET=your-secret-key > .env

# 2. 启动全部服务（PostgreSQL + SeaweedFS + 应用）
docker compose up -d
```

# Docker 单独构建

```bash
docker build -t vortex .
docker run -p 9178:9178 --env-file .env vortex
```


# 测试

```bash
# 单元测试
make test-unit

# 集成测试 (需要 Docker)
make test-integration

# 所有测试
make test-all
```

# 错误码

| 状态码 | 错误 | 说明 |
|--------|------|------|
| 400 | invalid_input | 请求参数无效 |
| 401 | unauthorized | 未授权 |
| 403 | forbidden | 无权限 |
| 404 | not_found | 资源不存在 |
| 409 | conflict | 资源冲突 |
| 429 | rate_limit_exceeded | 请求过于频繁 |
| 500 | internal_error | 服务器错误 |
| 503 | service_unavailable | 服务不可用 |

# 限流策略

| 端点 | 限制 |
|------|------|
| `/api/auth/login` | 5次失败/15分钟 |
| `/api/messages/send` | 1次/秒 |
| `/api/check` | 1次/3秒 |

# 数据验证

| 字段 | 规则 |
|------|------|
| username | 3-20字符，字母数字下划线 |
| password | 8-128字符，含大小写+数字+特殊字符(!@#$%^&*()) |
| email | 标准邮箱格式 |
| group name | 1-50字符 |
| message | 最大1000字符 |

# Systemd

```bash
sudo cp vortex.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable vortex
sudo systemctl start vortex
```

# 文档

- [openapi.yaml](openapi.yaml) - OpenAPI 3.0规范

# License

Apache 2.0
