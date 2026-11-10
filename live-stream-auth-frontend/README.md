# 直播互动演示前端

React 19、TypeScript、Vinext，配合 Beego 后端。包含账户、房间、弹幕、保存状态、同 UUID 手动重试、历史分页、点赞、在线数和热度榜。自动重连与完整增量补拉前端尚未交付。

## 启动

Node.js 版本要求见 package.json。联网环境首次安装：

```bash
npm ci
npm run dev
```

离线更新时保留既有 node_modules，只需 npm run dev。默认监听 http://127.0.0.1:13000，同源 /api 和 WebSocket 代理到 http://127.0.0.1:19000，浏览器携带 sid Cookie。

可在未提交的 .env.local 中设置 LIVE_STREAM_BACKEND_URL 后重启。当前后端 WS Origin 白名单使用前端 13000 地址，改变浏览器访问地址时需同步调整。

项目架构、完整启动步骤和接口见[根目录 README](../README.md)。本次 GitHub 发布整理未执行本机编译、类型检查或浏览器联调；Docker 工作暂缓。
