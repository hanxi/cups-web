# Passkey 登录

简体中文 · [English](en/passkey.md)

## 前置条件

- **HTTPS + 域名**：WebAuthn 要求安全上下文，RP ID 不能是 IP；仅 `localhost` 例外。
- 先用密码登录，再到「Passkey」页绑定。

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `PASSKEY_ENABLED` | 总开关，默认关闭。关闭时不注册 passkey 路由，前端不显示入口 |
| `PASSKEY_RP_ID` | 域名，如 `print.example.com`；启用时必填 |
| `PASSKEY_RP_ORIGINS` | 完整 origin，逗号分隔；缺省回退 `TRUSTED_ORIGINS` |
| `PASSKEY_RP_NAME` | 展示名，默认 `CUPS Web` |
| `DISABLE_PASSWORD_LOGIN` | 禁用密码登录；**至少一个管理员已绑定 Passkey 时才生效**，否则忽略，避免锁死 |

配置错误（缺 RP ID、RP ID 是 IP、缺 origin）会让进程启动失败并给出提示。

## 设计要点

- 可发现凭证：注册 `ResidentKey: required`，登录用 `BeginDiscoverableLogin`，点按钮后由浏览器列出账号，无需用户名。
- 校验通过后与密码登录走同一条 `auth.SetSession` + CSRF cookie 路径。
- begin → finish 的挑战保存在进程内存：5 分钟过期、一次性消费、上限 2000 条，经 `X-Passkey-Session` 头关联。
- 登录接口按 IP 复用登录限流；失败一律返回统一的 `passkey authentication failed`。
- 检测到克隆告警（签名计数回退）时拒绝登录；guest 不能绑定或用 Passkey 登录。
- 绑定/删除拒绝 API Key 通道；每人最多 10 个。
- 建议至少绑定两个 Passkey，并保留密码登录作为后备。
