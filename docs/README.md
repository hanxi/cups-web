# 深度文档索引

本目录收录 [AGENTS.md](../AGENTS.md) 中各章节的原理说明、故障案例与历史决策。AGENTS.md 本身保留可快速扫读的规则手册（API 表、DB 表、目录约定、退出码等），需要理解"为什么"时来此处查阅。

| 文件 | 内容 |
| --- | --- |
| [architecture.md](architecture.md) | 技术栈选型、外部依赖坑位（LibreOffice 可写 HOME、Ghostscript 字体破坏性改造、dpkg-dev 缺失） |
| [pdf-pipeline.md](pdf-pipeline.md) | PDF 标准化三链路、Ghostscript cidfmap 机制、自定义中文字体挂载与预检阻断、空壳 CJK 字体与 pdf.js 预览错位、HTTP 超时设计 |
| [driver-management.md](driver-management.md) | 驱动持久化原理、manifest 白名单翻车案例、AIO 单一 EXIT trap、退出码由来、架构探测、`.deb` 上传机制、`lpinfo` 解析、异步任务模型 |
| [container-startup.md](container-startup.md) | entrypoint 10 步流程、cupsd watchdog 子 shell 127 死循环、fast-fail 退避、restore-drivers 永远 exit 0 |
| [docker-build.md](docker-build.md) | AIO / 仅 Web 构建目标与共享依赖、三架构基础镜像选型史、ca-certificates 勿删、HOME/LibreOffice profile、docker-compose 配置理由、CI/CD |
| [reverse-proxy.md](reverse-proxy.md) | 反代下「登录提示密码错误」的三段成因、两类跨源 403 的分别修法、为什么默认信任 `X-Forwarded-Host`、cookie `Secure` 三态、Caddy/Traefik、子路径限制、限流与真实 IP |
| [conventions.md](conventions.md) | 常见开发任务步骤模板（新增 API/DB/页面/文件类型/驱动）、调试测试、Go/Vue 代码风格、Git 提交约定 |
| [printer-options.md](printer-options.md) | 驱动纸张、无边距、介质与质量选项，参数快照及照片队列批量执行（issue #122） |
