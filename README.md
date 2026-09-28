# Chat V0

自研即时通讯客服系统 V1。

目标：多个网站通过一行 JavaScript 接入客服 Widget，未来 Android 加速器 App 也接入同一套客服系统。客服端只开发 Android App，通过 Foreground Service + WebSocket + 固定 3 秒 HTTP 增量同步保证尽可能可靠地接收普通消息和紧急呼叫。

当前阶段：需求已冻结，先进行技术设计。暂不提交业务代码。

## 核心原则

- Site 是最高数据隔离边界。
- 同一 Site 内 `normalized_email` 唯一。
- 相同 Site 的 Web Widget 和未来 Android Customer SDK 共享聊天。
- XBoard 身份优先，XBoard 页面传来的用户资料视为可信。
- 所有消息先落库，WebSocket 只负责实时传输。
- Android Agent 固定每 3 秒 `/sync` 增量拉取作为兜底。
- 不接入 FCM、华为、小米、OPPO、vivo 或其他第三方 Push。
- 客服消息 5 分钟未被客户真正 READ 后，通过 Postal SMTP 合并提醒一次。

## 文档

- [V1 技术设计](docs/V1_TECHNICAL_DESIGN.md)
- [部署方案](DEPLOYMENT.md)
