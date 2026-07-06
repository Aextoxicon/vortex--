// 压力测试 - 测试系统在高负载下的表现
// 快速增加到高并发量，观察系统如何应对压力
// 面向 4 x vortex 实例 + HAProxy 负载均衡

import { group, sleep } from 'k6';
import { randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';
import {
  BASE_URL, uniqueUsername, registerPayload, loginPayload,
  createGroupPayload,
} from '../scripts/shared.js';
import * as api from '../scripts/shared.js';

export const options = {
  stages: [
    { target: 50, duration: '30s' },     // 热身：50 VUs
    { target: 200, duration: '1m' },      // 爬升：200 VUs
    { target: 500, duration: '2m' },      // 高负载：500 VUs
    { target: 800, duration: '2m' },      // 高压：800 VUs
    { target: 1000, duration: '2m' },     // 极限：1000 VUs
    { target: 0, duration: '1m' },        // 恢复：逐步降为 0
  ],
  thresholds: {
    http_req_duration: ['p(90)<5000', 'p(99)<10000'],
    http_req_failed: ['rate<0.10'],
  },
};

const userPool = [];

export default function () {
  const vuID = __VU;

  // 注册并登录（首次）
  if (!userPool[vuID]) {
    const username = uniqueUsername('stress');
    const regRes = api.registerUser(BASE_URL, registerPayload(username));
    if (regRes.status !== 201) {
      console.error(`[stress-VU${vuID}] register failed: ${regRes.status}`);
      return;
    }

    const logRes = api.loginUser(BASE_URL, loginPayload(username));
    if (logRes.status !== 200) {
      console.error(`[stress-VU${vuID}] login failed: ${logRes.status}`);
      return;
    }

    const body = JSON.parse(logRes.body);
    userPool[vuID] = {
      token: body.token,
      publicID: body.user.public_id,
    };

    sleep(randomIntBetween(0.5, 2));
  }

  const user = userPool[vuID];
  const action = Math.random();

  if (action < 0.35) {
    // 35% 概率：查询操作（读密集型）
    group('查询操作', () => {
      api.getMe(BASE_URL, user.token);
      api.getConversations(BASE_URL, user.token);
      api.getConversationCount(BASE_URL, user.token);
    });
  } else if (action < 0.50) {
    // 15% 概率：创建群组
    group('创建群组', () => {
      const payload = createGroupPayload(`stress-group-${vuID}-${Date.now()}`);
      api.createGroup(BASE_URL, user.token, payload);
    });
  } else if (action < 0.65) {
    // 15% 概率：消息操作（Get conversations + count）
    group('消息操作', () => {
      api.getConversations(BASE_URL, user.token);
    });
  } else if (action < 0.80) {
    // 15% 概率：更新用户信息
    group('更新操作', () => {
      const payload = JSON.stringify({ bio: `stress-update-${Date.now()}` });
      api.updateUser(BASE_URL, user.token, user.publicID, payload);
    });
  } else {
    // 20% 概率：登出 → 重新注册登录（模拟用户会话流转）
    group('认证轮换', () => {
      api.logoutUser(BASE_URL, user.token);
      const username = uniqueUsername('stress-relogin');
      const regRes = api.registerUser(BASE_URL, registerPayload(username));
      if (regRes.status === 201) {
        const logRes = api.loginUser(BASE_URL, loginPayload(username));
        if (logRes.status === 200) {
          const body = JSON.parse(logRes.body);
          userPool[vuID] = {
            token: body.token,
            publicID: body.user.public_id,
          };
        }
      }
    });
  }

  sleep(randomIntBetween(0.5, 2));
}