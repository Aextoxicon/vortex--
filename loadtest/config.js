export const BASE_URL = 'http://localhost:9178';

// 测试用例的请求参数
export const REGISTER_PASSWORD = 'test123456';

// 性能指标阈值
export const THRESHOLDS = {
  http_req_failed: ['rate<0.01'],      // 错误率 < 1%
  http_req_duration: ['p(95)<1000'],    // 95% 请求 < 1s
};
