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
├── shared.go            # 共享组件：通用结构体、健康检查、通知系统
├── worker.go            # 后台任务：定时任务、数据清理、表分区管理
└── test/                # 测试文件
    ├── validation_test.go  # 输入验证测试
    ├── ratelimit_test.go   # 限流测试
    ├── worker_test.go      # 后台任务测试
    ├── idgen_test.go       # ID 生成测试
    ├── jwt_test.go         # JWT 测试
    ├── store_test.go       # 数据访问测试
    ├── service_test.go     # 业务逻辑测试
    └── migration_test.go   # 数据库迁移测试
```