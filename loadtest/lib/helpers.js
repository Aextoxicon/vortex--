// 随机字符串
export function randomString(len = 8) {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < len; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}

// 生成唯一用户名（3-20字符，符合服务端校验规则）
export function uniqueUsername(prefix = 'test') {
  return `${prefix}_${randomString(8)}`;
}

// 生成随机消息内容
export function randomMessage() {
  const msgs = [
    '你好',
    '在吗？',
    '今天天气不错',
    '收到请回复',
    '测试消息内容',
    'Hello from k6',
    'This is a load test message',
    '压力测试中...',
  ];
  return msgs[Math.floor(Math.random() * msgs.length)];
}

// 生成私聊会话 ID (public_id 按字母序排列)
export function privateConvId(pid1, pid2) {
  const parts = [pid1, pid2].sort();
  return `p_${parts[0]}_${parts[1]}`;
}
