import http from 'k6/http';
import { check } from 'k6';
import { BASE_URL, REGISTER_PASSWORD } from '../config.js';

export function registerUser(username) {
  const res = http.post(`${BASE_URL}/api/auth/register`, JSON.stringify({
    username: username,
    password: REGISTER_PASSWORD,
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'register status 201': (r) => r.status === 201 });
  return res.status === 201
    ? { success: true, public_id: res.json('user.public_id'), username }
    : { success: false, username };
}

export function loginUser(username) {
  const res = http.post(`${BASE_URL}/api/auth/login`, JSON.stringify({
    username: username,
    password: REGISTER_PASSWORD,
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'login status 200': (r) => r.status === 200 });
  if (res.status === 200) {
    return { success: true, token: res.json('token'), public_id: res.json('user.public_id'), username };
  }
  return { success: false, username };
}

export function createSessionUser(username) {
  http.post(`${BASE_URL}/api/auth/register`, JSON.stringify({ username, password: REGISTER_PASSWORD }), { headers: { 'Content-Type': 'application/json' } });
  const loginRes = http.post(`${BASE_URL}/api/auth/login`, JSON.stringify({ username, password: REGISTER_PASSWORD }), { headers: { 'Content-Type': 'application/json' } });
  if (loginRes.status === 200) {
    return { success: true, token: loginRes.json('token'), public_id: loginRes.json('user.public_id'), username };
  }
  return { success: false, username };
}