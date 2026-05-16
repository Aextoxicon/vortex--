# Vortex API 文档

**基础URL**: `http://your-domain/api`  
**版本**: v1.0  
**认证方式**: JWT Bearer Token

---

# 目录

- [认证](#认证)
  - [注册](#注册)
  - [登录](#登录)
  - [获取当前用户](#获取当前用户)
  - [登出](#登出)
  - [更新用户](#更新用户)
  - [删除用户](#删除用户)
- [消息](#消息)
  - [发送消息](#发送消息)
  - [获取消息](#获取消息)
  - [撤回消息](#撤回消息)
  - [检查新消息](#检查新消息)
  - [获取会话列表](#获取会话列表)
- [好友](#好友)
  - [发送好友请求](#发送好友请求)
  - [获取好友请求列表](#获取好友请求列表)
  - [接受好友请求](#接受好友请求)
  - [拒绝好友请求](#拒绝好友请求)
  - [取消好友请求](#取消好友请求)
  - [拉黑用户](#拉黑用户)
  - [取消拉黑](#取消拉黑)
- [群组](#群组)
  - [创建群组](#创建群组)
  - [获取群组信息](#获取群组信息)
  - [更新群组](#更新群组)
  - [删除群组](#删除群组)
  - [加入群组](#加入群组)
  - [退出群组](#退出群组)
  - [踢出成员](#踢出成员)
- [文件](#文件)
  - [获取预签名URL](#获取预签名url)
- [健康检查](#健康检查)
  - [健康检查](#健康检查端点)
  - [就绪检查](#就绪检查)

---

# 认证

除注册和登录外，所有API都需要在请求头中携带JWT Token：

```
Authorization: Bearer <your_jwt_token>
```

# 注册

创建新用户账号。

**请求**
```
POST /api/auth/register
```

**请求体**
```json
{
  "username": "johndoe",
  "password": "SecurePass123!",
  "email": "john@example.com"
}
```

**参数说明**
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名，3-20字符，仅字母、数字、下划线 |
| password | string | 是 | 密码，8-72字符，需包含大小写字母、数字、特殊字符 |
| email | string | 否 | 邮箱地址（可选） |

**响应 201 Created**
```json
{
  "user": {
    "username": "johndoe",
    "email": "john@example.com",
    "public_id": "abc123XYZ"
  }
}
```

**错误响应**
- `400` - 无效输入、用户名格式错误、密码太弱、邮箱格式错误
- `409` - 用户名已存在

---

# 登录

用户登录获取JWT Token。

**请求**
```
POST /api/auth/login
```

**请求体**
```json
{
  "username": "johndoe",
  "password": "SecurePass123!"
}
```

**参数说明**
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |

**响应 200 OK**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "username": "johndoe",
    "email": "john@example.com",
    "public_id": "abc123XYZ"
  }
}
```

**错误响应**
- `401` - 用户名或密码错误
- `429` - 登录失败次数过多，请稍后重试（15分钟内最多5次）

**限流**: 15分钟内最多5次失败尝试

---

# 获取当前用户

获取当前登录用户信息。

**请求**
```
GET /api/auth/me
```

**认证**: 需要 Bearer Token

**响应 200 OK**
```json
{
  "user": {
    "username": "johndoe",
    "email": "john@example.com",
    "public_id": "abc123XYZ"
  }
}
```

**错误响应**
- `401` - 未授权

---

# 登出

登出当前用户，将Token加入黑名单。

**请求**
```
POST /api/auth/logout
```

**认证**: 需要 Bearer Token

**响应 200 OK**
```json
{
  "message": "logged out successfully"
}
```

---

# 更新用户

更新当前用户信息。

**请求**
```
PUT /api/auth/:publicId
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| publicId | string | 当前用户的Public ID |

**请求体**
```json
{
  "username": "newusername",
  "email": "newemail@example.com"
}
```

**参数说明**
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 否 | 新用户名 |
| email | string | 否 | 新邮箱 |

**响应 200 OK**
```json
{
  "user": {
    "username": "newusername",
    "email": "newemail@example.com",
    "public_id": "abc123XYZ"
  }
}
```

**错误响应**
- `400` - 用户名或邮箱格式错误
- `403` - 只能更新自己的信息
- `409` - 用户名已被使用

---

# 删除用户

删除当前用户账号。

**请求**
```
DELETE /api/auth/:publicId
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| publicId | string | 当前用户的Public ID |

**响应 200 OK**
```json
{
  "message": "user deleted successfully"
}
```

**错误响应**
- `403` - 只能删除自己的账号

---

# 消息

# 发送消息

发送消息给好友或群组。

**请求**
```
POST /api/messages/send
```

**认证**: 需要 Bearer Token

**限流**: 1秒1次

**请求体**
```json
{
  "conv_id": "conv_abc123",
  "content": "Hello, how are you?",
  "text": "Hello, how are you?",
  "content_type": "text",
  "client_msg_id": "msg_12345"
}
```

**参数说明**
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| conv_id | string | 是 | 会话ID |
| content | string | 是 | 消息内容（最大1000字符） |
| text | string | 否 | 纯文本内容（最大1000字符） |
| content_type | string | 否 | 内容类型：text, image, file等 |
| client_msg_id | string | 否 | 客户端消息ID（用于幂等性） |

**响应 201 Created**
```json
{
  "message": {
    "id": "msg_xyz789",
    "conv_id": "conv_abc123",
    "sender_id": "abc123XYZ",
    "content": "Hello, how are you?",
    "content_type": "text",
    "created_at": "2026-05-14T12:00:00Z"
  }
}
```

**错误响应**
- `400` - 无效输入、消息内容过长
- `403` - 不是会话参与者
- `429` - 发送过于频繁

**幂等性**: 如果提供`client_msg_id`且已存在，返回已有消息

---

# 获取消息

获取会话中的消息列表。

**请求**
```
GET /api/messages?conv_id=<conv_id>&page=<page>&page_size=<page_size>
```

**认证**: 需要 Bearer Token

**查询参数**
| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| conv_id | string | 是 | - | 会话ID |
| page | int | 否 | 1 | 页码 |
| page_size | int | 否 | 100 | 每页数量（最大500） |

**响应 200 OK**
```json
{
  "messages": [
    {
      "id": "msg_xyz789",
      "conv_id": "conv_abc123",
      "sender_id": "abc123XYZ",
      "content": "Hello!",
      "content_type": "text",
      "created_at": "2026-05-14T12:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 100,
  "has_more": true
}
```

---

# 撤回消息

撤回已发送的消息（2分钟内）。

**请求**
```
POST /api/messages/recall/:msgId
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| msgId | string | 消息ID |

**响应 200 OK**
```json
{
  "message": "message recalled successfully"
}
```

**错误响应**
- `400` - 消息已超过撤回时间限制（2分钟）
- `403` - 只能撤回自己的消息
- `404` - 消息不存在

---

# 检查新消息

检查是否有新消息。

**请求**
```
GET /api/check
```

**认证**: 需要 Bearer Token

**限流**: 3秒1次

**响应 200 OK**
```json
{
  "has_new": true,
  "unread_count": 5,
  "conversations": [
    {
      "conv_id": "conv_abc123",
      "unread": 3
    }
  ]
}
```

---

# 获取会话列表

获取用户的所有会话列表。

**请求**
```
GET /api/conversations
```

**认证**: 需要 Bearer Token

**响应 200 OK**
```json
{
  "conversations": [
    {
      "conv_id": "conv_abc123",
      "type": "private",
      "name": "John Doe",
      "last_message": {
        "content": "Hello!",
        "created_at": "2026-05-14T12:00:00Z"
      },
      "unread_count": 3
    }
  ]
}
```

---

# 好友

# 发送好友请求

向其他用户发送好友请求。

**请求**
```
POST /api/friends/request/send/:targetPublicId
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| targetPublicId | string | 目标用户的Public ID |

**响应 201 Created**
```json
{
  "id": 123,
  "status": "pending",
  "sender_public_id": "abc123XYZ",
  "receiver_public_id": "def456UVW"
}
```

**自动接受**: 如果对方已向你发送好友请求，会自动接受并返回`status: "auto_accepted"`

**错误响应**
- `400` - 不能向自己发送好友请求
- `404` - 目标用户不存在
- `409` - 已发送过好友请求或已是好友

---

# 获取好友请求列表

获取收到和发送的好友请求。

**请求**
```
GET /api/friends/requests
```

**认证**: 需要 Bearer Token

**响应 200 OK**
```json
{
  "sent": [
    {
      "id": 123,
      "to_user": {
        "username": "janedoe",
        "public_id": "def456UVW"
      },
      "status": "pending",
      "created_at": "2026-05-14T12:00:00Z"
    }
  ],
  "received": [
    {
      "id": 124,
      "from_user": {
        "username": "bobsmith",
        "public_id": "ghi789RST"
      },
      "status": "pending",
      "created_at": "2026-05-14T11:00:00Z"
    }
  ]
}
```

---

# 接受好友请求

接受收到的好友请求。

**请求**
```
POST /api/friends/request/:requestId/accept
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| requestId | int | 好友请求ID |

**响应 200 OK**
```json
{
  "message": "friend request accepted",
  "friend": {
    "username": "bobsmith",
    "public_id": "ghi789RST"
  }
}
```

**错误响应**
- `404` - 好友请求不存在
- `403` - 不是好友请求的接收者

---

# 拒绝好友请求

拒绝收到的好友请求。

**请求**
```
POST /api/friends/request/:requestId/reject
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| requestId | int | 好友请求ID |

**响应 200 OK**
```json
{
  "message": "friend request rejected"
}
```

---

# 取消好友请求

取消已发送的好友请求。

**请求**
```
DELETE /api/friends/request/:requestId
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| requestId | int | 好友请求ID |

**响应 200 OK**
```json
{
  "message": "friend request cancelled"
}
```

**错误响应**
- `403` - 不是好友请求的发送者

---

# 拉黑用户

拉黑指定用户。

**请求**
```
POST /api/blocks/:targetPublicId
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| targetPublicId | string | 目标用户的Public ID |

**响应 200 OK**
```json
{
  "message": "user blocked successfully"
}
```

---

# 取消拉黑

取消拉黑指定用户。

**请求**
```
DELETE /api/blocks/:targetPublicId
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| targetPublicId | string | 目标用户的Public ID |

**响应 200 OK**
```json
{
  "message": "user unblocked successfully"
}
```

---

# 群组

# 创建群组

创建新群组。

**请求**
```
POST /api/groups
```

**认证**: 需要 Bearer Token

**请求体**
```json
{
  "name": "My Group",
  "description": "A group for friends"
}
```

**参数说明**
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 群组名称（最大50字符） |
| description | string | 否 | 群组描述 |

**响应 201 Created**
```json
{
  "group": {
    "id": "grp_abc123",
    "name": "My Group",
    "description": "A group for friends",
    "owner_id": "abc123XYZ",
    "created_at": "2026-05-14T12:00:00Z"
  }
}
```

---

# 获取群组信息

获取群组详细信息。

**请求**
```
GET /api/groups/:id
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 群组ID |

**响应 200 OK**
```json
{
  "group": {
    "id": "grp_abc123",
    "name": "My Group",
    "description": "A group for friends",
    "owner_id": "abc123XYZ",
    "members": [
      {
        "public_id": "abc123XYZ",
        "username": "johndoe",
        "role": "owner"
      }
    ],
    "created_at": "2026-05-14T12:00:00Z"
  }
}
```

**错误响应**
- `404` - 群组不存在

---

# 更新群组

更新群组信息（仅群主可操作）。

**请求**
```
PUT /api/groups/:id
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 群组ID |

**请求体**
```json
{
  "name": "New Group Name"
}
```

**响应 200 OK**
```json
{
  "group": {
    "id": "grp_abc123",
    "name": "New Group Name",
    "description": "A group for friends"
  }
}
```

**错误响应**
- `403` - 不是群主

---

# 删除群组

删除群组（仅群主可操作）。

**请求**
```
DELETE /api/groups/:id
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 群组ID |

**响应 200 OK**
```json
{
  "message": "group deleted successfully"
}
```

**错误响应**
- `403` - 不是群主

---

# 加入群组

加入指定群组。

**请求**
```
POST /api/groups/:id/join
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 群组ID |

**响应 200 OK**
```json
{
  "message": "joined group successfully"
}
```

**错误响应**
- `404` - 群组不存在
- `409` - 已是群组成员

---

# 退出群组

退出群组。

**请求**
```
POST /api/groups/:id/leave
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 群组ID |

**响应 200 OK**
```json
{
  "message": "left group successfully"
}
```

---

# 踢出成员

将成员踢出群组（仅群主可操作）。

**请求**
```
DELETE /api/groups/:id/members/:memberPublicId
```

**认证**: 需要 Bearer Token

**路径参数**
| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 群组ID |
| memberPublicId | string | 成员的Public ID |

**响应 200 OK**
```json
{
  "message": "member kicked successfully"
}
```

**错误响应**
- `403` - 不是群主
- `404` - 成员不存在

---

# 文件

# 获取预签名URL

获取S3文件上传/下载的预签名URL。

**请求**
```
POST /api/files/presign
```

**认证**: 需要 Bearer Token

**请求体**
```json
{
  "operation": "upload",
  "conv_id": "conv_abc123",
  "file_ext": ".jpg"
}
```

或下载：

```json
{
  "operation": "download",
  "file_key": "files/conv_abc123/abc123.jpg"
}
```

**参数说明**
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| operation | string | 是 | 操作类型：upload 或 download |
| conv_id | string | 上传时必填 | 会话ID |
| file_ext | string | 上传时必填 | 文件扩展名 |
| file_key | string | 下载时必填 | 文件Key |

**响应 200 OK**
```json
{
  "url": "https://s3.amazonaws.com/bucket/...",
  "file_key": "files/conv_abc123/abc123.jpg",
  "expires_in": 3600
}
```

**错误响应**
- `400` - 无效操作类型
- `503` - S3服务未配置

---

# 健康检查

# 健康检查端点

检查服务是否运行。

**请求**
```
GET /health
```

**响应 200 OK**
```json
{
  "status": "ok"
}
```

---

# 就绪检查

检查服务是否就绪（数据库连接等）。

**请求**
```
GET /ready
```

**响应 200 OK**
```json
{
  "status": "ready"
}
```

**响应 503 Service Unavailable**
```json
{
  "status": "not ready"
}
```

---

# 错误响应格式

所有错误响应使用统一格式：

```json
{
  "error": "error_message"
}
```

# 常见错误码

| 状态码 | 错误信息 | 说明 |
|--------|---------|------|
| 400 | invalid_input | 请求参数无效 |
| 400 | invalid_username | 用户名格式错误 |
| 400 | invalid_email | 邮箱格式错误 |
| 400 | weak_password | 密码强度不足 |
| 401 | unauthorized | 未授权或Token无效 |
| 403 | forbidden | 无权限访问 |
| 404 | not_found | 资源不存在 |
| 409 | conflict | 资源冲突（如用户名已存在） |
| 429 | rate_limit_exceeded | 请求过于频繁 |
| 500 | internal_error | 服务器内部错误 |
| 503 | service_unavailable | 服务不可用 |

---

# 限流策略

| 端点 | 限制 | 时间窗口 |
|------|------|---------|
| POST /api/auth/login | 5次失败 | 15分钟 |
| POST /api/messages/send | 1次 | 1秒 |
| GET /api/check | 1次 | 3秒 |

---

# 数据验证规则

# 用户名
- 长度：3-20字符
- 允许：字母、数字、下划线
- 正则：`^[a-zA-Z0-9_]+$`

# 密码
- 长度：8-72字符
- 必须包含：
  - 至少1个大写字母
  - 至少1个小写字母
  - 至少1个数字
  - 至少1个特殊字符

# 邮箱
- 标准邮箱格式
- 最大长度：254字符

# 群组名称
- 长度：1-50字符
- 允许：字母、数字、空格、下划线、连字符、#

# 消息内容
- 最大长度：1000字符

---

# WebSocket (未来支持)

计划支持的实时通信功能：
- 新消息推送
- 好友在线状态
- 正在输入提示
- 消息已读回执

---

# 版本历史

- **v1.0** (2026-05-14)
  - 初始版本
  - 用户认证
  - 消息收发
  - 好友系统
  - 群组功能
  - 文件上传（S3）
