export const BASE_URL = __ENV.K6_BASE_URL || 'http://localhost:9178';

// Test user password (must match server rules: lowercase+uppercase+digit+special, >=8 chars)
export const REGISTER_PASSWORD = 'Test1234!';

// Performance threshold values
export const THRESHOLDS = {
  http_req_failed: ['rate<0.01'],
  http_req_duration: ['p(95)<1000'],
};