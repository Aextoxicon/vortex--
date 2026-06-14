import http from 'k6/http';
import { sleep } from 'k6';
import { BASE_URL } from './config.js';
import { createSessionUser } from './lib/setup.js';
import { uniqueUsername } from './lib/helpers.js';

export const options = {
  stages: [
    { duration: '10s', target: 300 },
    { duration: '30s', target: 300 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<3000'],
  },
};

const users = {};

export default function () {
  if (!users[__VU]) {
    const user = createSessionUser(uniqueUsername('spike'));
    if (!user.success) return;
    users[__VU] = user;
  }
  const user = users[__VU];
  const headers = {
    'Authorization': `Bearer ${user.token}`,
    'Content-Type': 'application/json',
  };

  http.get(`${BASE_URL}/api/auth/me`, { headers });
  http.get(`${BASE_URL}/api/conversations`, { headers });

  sleep(1);
}