import { check, sleep } from 'k6';
import http from 'k6/http';
import { SharedArray } from 'k6/data';

// ==================== 配置 ====================

// 默认基础 URL，可通过环境变量覆盖
export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// 测试用户池大小
export const USER_POOL_SIZE = parseInt(__ENV.USER_POOL_SIZE || '50', 10);

// ==================== 辅助函数 ====================

// 生成唯一用户名
let userCounter = 0;
export function uniqueUsername(prefix = 'k6user') {
  userCounter++;
  const ts = Date.now().toString(36).slice(-6);
  return `${prefix}_${ts}_${userCounter}`;
}

// 默认测试用密码
export const DEFAULT_PASSWORD = 'Test1234!';

// 生成测试用户注册 payload
export function registerPayload(username) {
  return JSON.stringify({
    username: username,
    password: DEFAULT_PASSWORD,
    email: `${username}@k6test.com`,
    bio: 'k6 load test user',
  });
}

// 生成登录 payload
export function loginPayload(username) {
  return JSON.stringify({
    username: username,
    password: DEFAULT_PASSWORD,
  });
}

// 发送消息 payload
export function messagePayload(convID, content = 'Hello from k6!') {
  return JSON.stringify({
    conv_id: convID,
    content: content,
  });
}

// 创建群组 payload
export function createGroupPayload(name, description = '') {
  return JSON.stringify({
    name: name,
    description: description,
  });
}

// ==================== HTTP 请求封装 ====================

// 健康检查
export function healthCheck(baseURL = BASE_URL) {
  const res = http.get(`${baseURL}/health`);
  check(res, {
    'health check status is 200': (r) => r.status === 200,
    'health check body has ok status': (r) => JSON.parse(r.body).status === 'ok',
  });
  return res;
}

// 就绪检查
export function readinessCheck(baseURL = BASE_URL) {
  const res = http.get(`${baseURL}/ready`);
  check(res, {
    'readiness check is 200 or 503': (r) => r.status === 200 || r.status === 503,
  });
  return res;
}

// 用户注册
export function registerUser(baseURL, payload) {
  const params = {
    headers: { 'Content-Type': 'application/json' },
  };
  const res = http.post(`${baseURL}/api/auth/register`, payload, params);
  check(res, {
    'register status is 201': (r) => r.status === 201,
  });
  return res;
}

// 用户登录
export function loginUser(baseURL, payload) {
  const params = {
    headers: { 'Content-Type': 'application/json' },
  };
  const res = http.post(`${baseURL}/api/auth/login`, payload, params);
  check(res, {
    'login status is 200': (r) => r.status === 200,
    'login returns token': (r) => JSON.parse(r.body).token !== undefined,
  });
  return res;
}

// 获取当前用户信息（带认证）
export function getMe(baseURL, token) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.get(`${baseURL}/api/auth/me`, params);
  check(res, {
    'get me status is 200': (r) => r.status === 200,
  });
  return res;
}

// 发送消息
export function sendMessage(baseURL, token, payload) {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
  };
  const res = http.post(`${baseURL}/api/messages/send`, payload, params);
  check(res, {
    'send message status is 201': (r) => r.status === 201,
  });
  return res;
}

// 获取会话列表
export function getConversations(baseURL, token) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.get(`${baseURL}/api/conversations`, params);
  check(res, {
    'get conversations status is 200': (r) => r.status === 200,
  });
  return res;
}

// 获取会话数量
export function getConversationCount(baseURL, token) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.get(`${baseURL}/api/conversations/count`, params);
  check(res, {
    'get conversation count status is 200': (r) => r.status === 200,
  });
  return res;
}

// 获取消息详情
export function getMessageByID(baseURL, token, msgId) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.get(`${baseURL}/api/messages/${msgId}`, params);
  check(res, {
    'get message status is 200': (r) => r.status === 200,
  });
  return res;
}

// 撤回消息
export function recallMessage(baseURL, token, msgId) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.post(`${baseURL}/api/messages/recall/${msgId}`, null, params);
  check(res, {
    'recall message status is 200': (r) => r.status === 200,
  });
  return res;
}

// 发送好友请求
export function sendFriendRequest(baseURL, token, targetPublicID) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.post(
    `${baseURL}/api/friends/request/send/${targetPublicID}`,
    null,
    params
  );
  check(res, {
    'send friend request status is 201': (r) => r.status === 201,
  });
  return res;
}

// 获取好友请求列表
export function getFriendRequests(baseURL, token) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.get(`${baseURL}/api/friends/requests`, params);
  check(res, {
    'get friend requests status is 200': (r) => r.status === 200,
  });
  return res;
}

// 接受好友请求
export function acceptFriendRequest(baseURL, token, requestId) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.post(
    `${baseURL}/api/friends/request/${requestId}/accept`,
    null,
    params
  );
  check(res, {
    'accept friend request status is 200': (r) => r.status === 200,
  });
  return res;
}

// 取消好友请求
export function cancelFriendRequest(baseURL, token, requestId) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.del(
    `${baseURL}/api/friends/request/${requestId}`,
    null,
    params
  );
  check(res, {
    'cancel friend request status is 204': (r) => r.status === 204,
  });
  return res;
}

// 创建群组
export function createGroup(baseURL, token, payload) {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
  };
  const res = http.post(`${baseURL}/api/groups`, payload, params);
  check(res, {
    'create group status is 201': (r) => r.status === 201,
  });
  return res;
}

// 获取群组详情
export function getGroup(baseURL, token, groupID) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.get(`${baseURL}/api/groups/${groupID}`, params);
  check(res, {
    'get group status is 200': (r) => r.status === 200,
  });
  return res;
}

// 加入群组
export function joinGroup(baseURL, token, groupID) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.post(
    `${baseURL}/api/groups/${groupID}/join`,
    null,
    params
  );
  check(res, {
    'join group status is 200': (r) => r.status === 200,
  });
  return res;
}

// 离开群组
export function leaveGroup(baseURL, token, groupID) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.post(
    `${baseURL}/api/groups/${groupID}/leave`,
    null,
    params
  );
  check(res, {
    'leave group status is 200': (r) => r.status === 200,
  });
  return res;
}

// 阻塞用户
export function blockUser(baseURL, token, targetPublicID) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.post(
    `${baseURL}/api/blocks/${targetPublicID}`,
    null,
    params
  );
  check(res, {
    'block user status is 200': (r) => r.status === 200,
  });
  return res;
}

// 解除阻塞
export function unblockUser(baseURL, token, targetPublicID) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.del(
    `${baseURL}/api/blocks/${targetPublicID}`,
    null,
    params
  );
  check(res, {
    'unblock user status is 200': (r) => r.status === 200,
  });
  return res;
}

// 更新用户信息
export function updateUser(baseURL, token, publicID, payload) {
  const params = {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
  };
  const res = http.put(`${baseURL}/api/auth/${publicID}`, payload, params);
  check(res, {
    'update user status is 200': (r) => r.status === 200,
  });
  return res;
}

// 用户登出
export function logoutUser(baseURL, token) {
  const params = {
    headers: { Authorization: `Bearer ${token}` },
  };
  const res = http.post(`${baseURL}/api/auth/logout`, null, params);
  check(res, {
    'logout status is 200': (r) => r.status === 200,
  });
  return res;
}