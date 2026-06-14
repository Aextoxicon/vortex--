import http from 'k6/http';
import { sleep } from 'k6';
import { BASE_URL } from './config.js';
import { createSessionUser } from './lib/setup.js';
import { uniqueUsername } from './lib/helpers.js';

export const options = {
  vus: 50,
  duration: '30m',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<2000'],
  },
};

const users = {};

export default function () {
  if (!users[__VU]) {
    const user = createSessionUser(uniqueUsername('soak'));
    if (!user.success) return;
    users[__VU] = user;
  }
  const user = users[__VU];
  const headers = {
    'Authorization': `Bearer ${user.token}`,
    'Content-Type': 'application/json',
  };

  const op = Math.random();
  if (op < 0.5) {
    http.get(`${BASE_URL}/api/auth/me`, { headers });
  } else if (op < 0.8) {
    http.get(`${BASE_URL}/api/conversations`, { headers });
  } else {
    http.get(`${BASE_URL}/health`);
  }

  sleep(3 + Math.random() * 2);
}