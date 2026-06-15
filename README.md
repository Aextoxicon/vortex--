# Vortex

AI写代码太好用了xdm

# 项目结构

```
vortex/
├── src/
│   ├── main.rs            # 应用入口：启动服务、初始化、路由注册
│   ├── config.rs          # 配置管理：环境变量加载与验证
│   ├── error.rs           # 错误处理：自定义错误类型和响应格式
│   ├── account.rs         # 用户模块：注册/登录/用户信息管理
│   ├── messaging.rs       # 消息模块：发送/接收/撤回消息、会话列表
│   ├── friend.rs          # 好友模块：好友请求/接受/拒绝/列表
│   ├── groups.rs          # 群组模块：创建群组/邀请/群组管理
│   ├── jwt.rs             # 认证模块：JWT Token 生成/验证/黑名单
│   ├── ratelimit.rs       # 限流模块：请求频率控制、防滥用
│   ├── idgen.rs           # ID 生成：分布式唯一 ID 生成器
│   ├── migration.rs       # 数据库迁移：表结构创建和版本管理
│   ├── store.rs           # 数据访问层：所有数据库 CRUD 操作
│   ├── s3.rs              # 文件模块：S3 预签名 URL 生成、文件权限控制
│   ├── shared.rs          # 共享组件：Service结构体、健康检查、通知系统
│   ├── worker.rs          # 后台任务：定时任务、数据清理、表分区管理
│   └── metrics.rs         # 指标模块：性能监控和统计
├── tests/                 # 集成测试
│   ├── handler_test.rs    # HTTP 端点测试
│   ├── service_test.rs    # 业务逻辑测试
│   ├── store_test.rs      # 数据访问层测试
│   ├── jwt_test.rs        # JWT 认证测试
│   ├── ratelimit_test.rs  # 限流器测试
│   ├── validation_test.rs # 验证逻辑测试
│   ├── worker_test.rs     # 后台任务测试
│   ├── migration_test.rs  # 数据库迁移测试
│   ├── idgen_test.rs      # ID 生成器测试
│   └── test_utils.rs      # 测试工具
├── Cargo.toml             # Rust 依赖管理
├── Cargo.lock             # 锁定依赖版本
└── Makefile               # 构建和测试命令
```

# 快速开始

# 环境要求

- Rust 1.85+
- PostgreSQL 16+
- Docker (可选，用于测试)
- S3 服务

# 安装

```bash
git clone https://github.com/Lwh20/vortex--.git
cd vortex--
cargo build --release
mkdir -p /opt/vortex/
cp target/release/vortex /opt/vortex/
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

# 测试

```bash
# 单元测试
make test-unit

# 集成测试 (需要 Docker)
make test-integration

# 所有测试
make test-all
```

# API 快速参考

**认证**: `Authorization: Bearer <token>`

# 端点速查表

# 认证 (Auth)

| 方法 | 端点 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/auth/register` | 否 | 注册新用户 |
| POST | `/api/auth/login` | 否 | 登录获取Token |
| GET | `/api/auth/me` | 是 | 获取当前用户 |
| POST | `/api/auth/logout` | 是 | 登出 |
| PUT | `/api/auth/:publicId` | 是 | 更新用户信息 |
| DELETE | `/api/auth/:publicId` | 是 | 删除用户 |

# 消息 (Messages)

| 方法 | 端点 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/messages/send` | 是 | 发送消息 |
| GET | `/api/messages` | 是 | 获取消息列表 |
| GET | `/api/messages/:msgId` | 是 | 获取消息详情 |
| POST | `/api/messages/recall/:msgId` | 是 | 撤回消息 |
| GET | `/api/check` | 是 | 检查新消息 |
| GET | `/api/conversations` | 是 | 获取会话列表 |
| GET | `/api/conversations/count` | 是 | 获取用户会话数 |
| GET | `/api/conversations/:convId/participants` | 是 | 获取会话参与者 |
| GET | `/api/conversations/:convId/blocked/:userId` | 是 | 检查屏蔽状态 |

# 好友 (Friends)

| 方法 | 端点 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/friends/request/send/:targetPublicId` | 是 | 发送好友请求 |
| GET | `/api/friends/requests` | 是 | 获取好友请求 |
| GET | `/api/friends/requests/pending` | 是 | 获取待处理好友请求 |
| POST | `/api/friends/request/:requestId/accept` | 是 | 接受请求 |
| POST | `/api/friends/request/:requestId/reject` | 是 | 拒绝请求 |
| DELETE | `/api/friends/request/:requestId` | 是 | 取消请求 |
| POST | `/api/friends/requests/pending` | 是 | 获取待处理好友请求 |
| POST | `/api/blocks/:targetPublicId` | 是 | 拉黑用户 |
| DELETE | `/api/blocks/:targetPublicId` | 是 | 取消拉黑 |

# 群组 (Groups)

| 方法 | 端点 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/groups` | 是 | 创建群组 |
| GET | `/api/groups/:id` | 是 | 获取群组信息 |
| PUT | `/api/groups/:id` | 是 | 更新群组 |
| DELETE | `/api/groups/:id` | 是 | 删除群组 |
| POST | `/api/groups/:id/join` | 是 | 加入群组 |
| POST | `/api/groups/:id/leave` | 是 | 退出群组 |
| DELETE | `/api/groups/:id/members/:memberPublicId` | 是 | 踢出成员 |
| GET | `/api/groups/:id/members/count` | 是 | 获取群组成员数 |

# 文件 (Files)

| 方法 | 端点 | 认证 | 说明 |
|------|------|------|------|
| POST | `/api/files/presign` | 是 | 获取预签名URL |

# 健康检查 (Health)

| 方法 | 端点 | 认证 | 说明 |
|------|------|------|------|
| GET | `/health` | 否 | 健康检查 |
| GET | `/ready` | 否 | 就绪检查 |
| GET | `/metrics` | 否 | 运行时指标（PID、线程数、内存） |

# 请求示例

注册：
```bash
curl -X POST http://localhost:9178/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"johndoe","password":"SecurePass123!","email":"john@example.com"}'
```

登录：
```bash
curl -X POST http://localhost:9178/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"johndoe","password":"SecurePass123!"}'
```

发送消息：
```bash
curl -X POST http://localhost:9178/api/messages/send \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"conv_id":"conv_abc123","content":"Hello!"}'
```

获取消息：
```bash
curl -X GET "http://localhost:9178/api/messages?conv_id=conv_abc123&page=1&page_size=100" \
  -H "Authorization: Bearer <token>"
```

发送好友请求：
```bash
curl -X POST http://localhost:9178/api/friends/request/send/def456UVW \
  -H "Authorization: Bearer <token>"
```

创建群组：
```bash
curl -X POST http://localhost:9178/api/groups \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"My Group","description":"A group for friends"}'
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
| username | 3-20字符，字母数字下划线及中日韩字符 |
| password | 8-128字符，含大小写+数字+特殊字符(!@#$%^&*()) |
| email | 标准邮箱格式，最大100字符 |
| group name | 1-50字符 |
| message | 最大1000字符 |

# 部署

# Systemd

```bash
sudo cp vortex.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable vortex
sudo systemctl start vortex
```

# 文档

- [API.md](API.md) - 完整API文档
- [openapi.yaml](openapi.yaml) - OpenAPI 3.0规范
- [postman_collection.json](postman_collection.json) - Postman集合

# License

Apache 2.0
