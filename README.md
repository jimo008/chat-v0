# Chat V0

自研即时通讯客服系统 V1。

目标：多个网站通过一行 JavaScript 接入客服 Widget，未来 Android 加速器 App 也接入同一套客服系统。客服端只开发 Android App，通过 Foreground Service + WebSocket + 固定 3 秒 HTTP 增量同步保证尽可能可靠地接收普通消息和紧急呼叫。

当前状态：后端基础链路、Widget 基础聊天、Postal Worker、紧急呼叫状态机、Android Agent 基础工程正在开发中。

## 核心原则

- Site 是最高数据隔离边界。
- 同一 Site 内 `normalized_email` 唯一。
- 相同 Site 的 Web Widget 和未来 Android Customer SDK 共享聊天。
- XBoard 身份优先，XBoard 页面传来的用户资料视为可信。
- 所有消息先落库，WebSocket 只负责实时传输。
- Android Agent 固定每 3 秒 `/sync` 增量拉取作为兜底。
- 不接入 FCM、华为、小米、OPPO、vivo 或其他第三方 Push。
- 客服消息 5 分钟未被客户真正 READ 后，通过 Postal SMTP 合并提醒一次。

## Quick Start

```bash
git clone https://github.com/jimo008/chat-v0.git
cd chat-v0
chmod +x supportctl
./supportctl install
# edit .env: APP_BASE_URL, MySQL passwords, Postal SMTP
./supportctl up
./supportctl status
```

Create a Site after the stack is ready:

```bash
./supportctl site create
./supportctl site list
./supportctl site code st_xxxxxxxx
```

## 文档

- [V1 技术设计](docs/V1_TECHNICAL_DESIGN.md)
- [部署方案](DEPLOYMENT.md)
- [开发说明](docs/DEVELOPMENT.md)
- [API Reference](docs/API.md)

## 已实现的基础能力

- Site 创建和部署代码生成
- 第一位客服账号初始化
- 客服登录、token 鉴权、3 秒 `/sync`、Agent WebSocket
- XBoard 可信登录和邮箱验证码登录
- 客户/客服文字消息
- 客户/客服图片上传基础版
- 客户 READ 上报
- 紧急呼叫发起、取消、接受、前台自动接通、超时过期
- Postal 5 分钟未读邮件批次 Worker
- 三个月消息和附件清理 Worker（默认关闭，需显式开启）
- Web Widget 基础聊天界面
- Android Agent App 基础工程
