# Vortex 压测脚本 (k6)

## 目录结构

`
loadtest/
├── config.js            # 全局配置（BASE_URL、阈值等）
├── lib/
│   ├── setup.js         # 批量注册/登录用户 + Token 管理
│   └── helpers.js       # 随机数据生成等工具函数
├── smoke.js             # 冒烟测试（小并发，验证功能正常）
├── stress.js            # 压力测试（逐步加压，找瓶颈）
├── spike.js             # 尖峰测试（突发高并发）
├── soak.js              # 耐力测试（长时间运行，检查稳定性）
└── README.md
`

## 安装 k6

- **Windows**: winget install k6 或下载 exe
- **macOS**: rew install k6
- **Linux**: 从 [k6.io](https://k6.io/docs/getting-started/installation/) 下载

## 使用

`ash
cd loadtest

# 冒烟测试
k6 run smoke.js

# 压力测试
k6 run stress.js

# HTML 报告
k6 run --out html=report.html stress.js
`

## 场景说明

| 脚本 | 并发 | 时长 | 目的 |
|------|------|------|------|
| smoke.js | 1~2 VUs | 30s | 验证所有 API 正常 |
| stress.js | 1→100→0 | 3min | 找出系统瓶颈 |
| spike.js | 0→200→0 | 2min | 测试突发流量 |
| soak.js | 50 const | 30min | 检查内存泄漏/稳定性 |
