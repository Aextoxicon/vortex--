import http from 'k6/http';
import { check, sleep } from 'k6';
import { BASE_URL, THRESHOLDS } from './config.js';
import { createSessionUser } from './lib/setup.js';
import { uniqueUsername } from './lib/helpers.js';

export const options = {
  vus: 1,
  duration: '30s',
  thresholds: THRESHOLDS,
};

export default function () {
  const healthRes = http.get(`${BASE_URL}/health`);
  check(healthRes, { 'health endpoint returns 200': (r) => r.status === 200 });

  const user = createSessionUser(uniqueUsername('smoke'));
  if (!user.success) return;

  const headers = {
    'Authorization': `Bearer ${user.token}`,
    'Content-Type': 'application/json',
  };

  const meRes = http.get(`${BASE_URL}/api/auth/me`, { headers });
  check(meRes, { 'get /me returns 200': (r) => r.status === 200 });

  const convRes = http.get(`${BASE_URL}/api/conversations`, { headers });
  check(convRes, { 'get conversations returns 200': (r) => r.status === 200 });

  const health2 = http.get(`${BASE_URL}/health`);
  check(health2, { 'health still works': (r) => r.status === 200 });

  sleep(1);
}