# scan - 扫描仪网络配置

本目录用于配置 SANE 网络扫描仪。容器启动时，entrypoint 脚本会读取 `config.json` 并自动修改 `/etc/sane.d/` 下对应的配置文件。

## 配置文件格式

`config.json` 示例：

```json
{
  "scanners": [
    {
      "backend": "epsonds",
      "ip": "192.168.1.11"
    },
    {
      "backend": "epson2",
      "ip": "192.168.1.22"
    }
  ]
}
```

### 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `backend` | 是 | SANE 后端名称，对应 `/etc/sane.d/<backend>.conf` |
| `ip` | 是 | 扫描仪局域网 IP 地址 |

脚本会在 `/etc/sane.d/<backend>.conf` 末尾追加 `net <ip>` 行，同时在 `/etc/sane.d/net.conf` 中也追加相同条目。

### 常见后端名称

| 后端 | 适用型号 |
|------|----------|
| `epsonds` | Epson DS 系列（DS-1660W、DS-530 等）网络扫描仪 |
| `epson2` | Epson 其他网络扫描仪（GT、Perfection 等） |
| `pixma` | Canon PIXMA 系列（注意：Canon 使用 `bjnp://<ip>` 格式，当前不支持） |
| `fujitsu` | Fujitsu fi 系列扫描仪 |
| `brother` | Brother 网络扫描仪 |
| `airscan` | 支持 eSCL/WSD 协议的现代扫描仪（自动发现，通常无需手动配置） |

## 使用方式

### 方式一：构建时固定配置

编辑 `config.json` 后执行 `docker build` 或 `make docker-build`，配置会写入镜像。

### 方式二：运行时挂载（推荐）

`docker-compose.yml` 已配置 `./scan:/scan` 挂载，修改 `config.json` 后重启容器即可生效：

```bash
# 编辑配置
vim scan/config.json

# 重启 web 容器
docker compose restart web
```

## 注意事项

- 容器需要以 **root** 用户运行（`docker-compose.yml` 已配置 `user: root`，直接 `docker run` 时需传 `--user root`）
- IP 地址变更后需要重启容器才能生效
- 如果 `config.json` 的 `scanners` 数组为空，不会修改任何配置文件
- 重复的 `net <ip>` 行会被自动跳过（去重）
- 当前仅支持 `net <ip>` 格式（适用于 epsonds、epson2、fujitsu 等后端）
