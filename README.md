# 网页打印机

方便在浏览器里远程控制家里的打印机打印东西，解决远程桌面操作麻烦的问题。

## 技术栈

- 打印服务 https://github.com/OpenPrinting/cups
- Go 后端
- Vue 前端

# 使用 docker 部署

cups 服务建议根据自己的打印机型号安装合适的驱动。


SESSION_HASH_KEY 和 SESSION_BLOCK_KEY 可以用下面的命令生成
```
openssl rand -base64 32 | tr -d '\n'
```
