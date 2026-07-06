// 尖峰测试 - 模拟突发流量高峰
// 短时间内 VUs 激增，测试系统弹性
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
    { target: 20, duration: '30s' },     // 热身：20 VUs
    { target: 20, duration: '1m' },       // 基线：20 VUs
    { target: 1000, duration: '10s' },    // 尖峰：10s 内飙升到 1000 VUs
    { target: 1000, duration: '30s' },    // 保持尖峰 30s
    { target: 20, duration: '30s' },      // 恢复：回到 20 VUs
    { target: 20, duration: '1m' },       // 恢复期观察
  ],
  thresholds: {
    http_req_duration: ['p(90)<8000', 'p(99)<15000'],
    http_req_failed: ['rate<0.15'],
  },
};

const userPool = [];

export default function () {
  const vuID = __VU;

  if (!userPool[vuID]) {
    const username = uniqueUsername('spike');
    const regRes = api.registerUser(BASE_URL, registerPayload(username));
    if (regRes.status !== 201) {
      return;
    }

    const logRes = api.loginUser(BASE_URL, loginPayload(username));
    if (logRes.status !== 200) {
      return;
    }

    const body = JSON.parse(logRes.body);
    userPool[vuID] = {
      token: body.token,
      publicID: body.user.public_id,
    };

    sleep(randomIntBetween(0.3, 1));
  }

  const user = userPool[vuID];
  const action = Math.random();

  if (action < 0.40) {
    // 40% 概率：读取操作（峰值时优先保读）
    group('读取操作', () => {
      api.getMe(BASE_URL, user.token);
      api.getConversations(BASE_URL, user.token);
      api.getConversationCount(BASE_URL, user.token);
    });
  } else if (action < 0.60) {
    // 20% 概率：创建群组
    group('创建群组', () => {
      const payload = createGroupPayload(`spike-group-${vuID}`);
      api.createGroup(BASE_URL, user.token, payload);
    });
  } else if (action < 0.80) {
    // 20% 概率：用户更新
    group('更新操作', () => {
      const payload = JSON.stringify({ bio: `spike-update-${Date.now()}` });
      api.updateUser(BASE_URL, user.token, user.publicID, payload);
    });
  } else {
    // 20% 概率：新用户注册（模拟尖峰时的新用户涌入）
    group('新用户注册', () => {
      const username = uniqueUsername('spike-auth');
      api.registerUser(BASE_URL, registerPayload(username));
    });
  }

  sleep(randomIntBetween(0.2, 1));
}