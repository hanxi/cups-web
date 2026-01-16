# 🖨️ CUPS Web - 网页打印机

[![Docker Pulls](https://img.shields.io/docker/pulls/hanxi/cups-web?style=flat-square&logo=docker)](https://hub.docker.com/r/hanxi/cups-web)
[![GitHub Stars](https://img.shields.io/github/stars/hanxi/cups-web?style=flat-square&logo=github)](https://github.com/hanxi/cups-web)
[![License](https://img.shields.io/github/license/hanxi/cups-web?style=flat-square)](LICENSE)

这是一个功能完善的网页版打印机管理工具。它允许你通过浏览器远程控制打印机，支持多用户管理、余额充值、打印记录追踪等企业级功能，轻松实现家庭或小型办公室的打印管理需求。

## 📸 界面预览

<div align="center">

<table>
  <tr>
    <td align="center">
      <img src="screenshots/login.png" width="400" alt="登录认证"><br/>
      <b>登录认证</b>
    </td>
    <td align="center">
      <img src="screenshots/print1.png" width="400" alt="打印机列表"><br/>
      <b>打印机列表</b>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="screenshots/print2.png" width="400" alt="文件上传"><br/>
      <b>文件上传</b>
    </td>
    <td align="center">
      <img src="screenshots/admin.png" width="400" alt="管理后台"><br/>
      <b>管理后台</b>
    </td>
  </tr>
</table>

</div>

## ✨ 功能特点

### 核心功能
- **远程打印**：随时随地通过网页上传文件进行打印
- **多格式支持**：
  - PDF 文档
  - 图片文件（JPG、PNG、GIF）
  - Office 文档（docx、xlsx、pptx 等）自动转换为 PDF（基于 LibreOffice）
  - 文本文件（txt）自动转换为 PDF

### 用户管理
- **多用户系统**：支持管理员和普通用户两种角色
- **余额管理**：每个用户独立的打印余额账户
- **自动充值**：支持每日/每月/每年自动充值
- **消费限额**：可设置月度和年度打印消费限额
- **打印记录**：完整的打印历史记录和费用追踪

### 管理后台
- **用户管理**：创建、编辑、删除用户账号
- **充值管理**：为用户手动充值，查看充值记录
- **打印记录查询**：按用户、时间范围查询打印记录
- **系统设置**：配置单页打印价格、数据保留天数等

### 安全特性
- **Session 认证**：安全的会话管理机制
- **CSRF 保护**：防止跨站请求伪造攻击
- **密码加密**：使用 bcrypt 加密存储用户密码

### 部署优势
- **Docker 一键部署**：使用 Docker Compose 快速启动
- **数据持久化**：数据库和上传文件独立存储
- **易于维护**：简洁的配置和管理界面

## 🛠️ 技术栈

- **打印服务**: [CUPS](https://github.com/OpenPrinting/cups)
- **后端**: Go
- **前端**: Vue.js

## 🚀 快速开始

### 前置要求

- Docker
- Docker Compose
- USB 打印机（如果使用本地打印机）

### 1. 创建项目目录

```bash
mkdir cups-web
cd cups-web
```

### 2. 创建 docker-compose.yml

创建 `docker-compose.yml` 文件，内容如下：

```yaml
services:
  cups:
    image: hanxi/cups:latest
    environment:
      - CUPSADMIN=${CUPSADMIN}
      - CUPSPASSWORD=${CUPSPASSWORD}
    ports:
      - "631:631"
    devices:
      - /dev/bus/usb:/dev/bus/usb
    volumes:
      - ./.etc:/etc/cups
    restart: unless-stopped

  web:
    image: hanxi/cups-web:latest
    environment:
      - SESSION_HASH_KEY=${SESSION_HASH_KEY}
      - SESSION_BLOCK_KEY=${SESSION_BLOCK_KEY}
      - SESSION_SECURE=${SESSION_SECURE}
      - CUPS_HOST=cups:631
    volumes:
      - ./.data:/data
      - ./.uploads:/uploads
    ports:
      - "1180:8080"
    depends_on:
      - cups
    restart: unless-stopped
```

或者直接下载：

```bash
wget https://raw.githubusercontent.com/hanxi/cups-web/main/docker-compose.yml
```

### 3. 配置环境变量

创建 `.env` 文件并配置以下环境变量：

```bash
# CUPS 管理员账号（用于管理打印机）
CUPSADMIN=admin
CUPSPASSWORD=your_cups_password

# Session 加密密钥（必须配置，用于保护用户会话）
SESSION_HASH_KEY=your_hash_key_here
SESSION_BLOCK_KEY=your_block_key_here

# 如果使用 HTTPS，设置为 true
SESSION_SECURE=false
```

**生成安全的密钥：**

```bash
# 生成 SESSION_HASH_KEY
openssl rand -base64 32 | tr -d '\n'

# 生成 SESSION_BLOCK_KEY
openssl rand -base64 32 | tr -d '\n'
```

### 4. 启动服务

```bash
docker-compose up -d
```

### 5. 配置打印机

访问 CUPS 管理界面配置打印机：

```
http://localhost:631
```

使用 `.env` 中配置的 `CUPSADMIN` 和 `CUPSPASSWORD` 登录，然后添加你的打印机。

**提示**：建议根据打印机型号安装合适的驱动程序。

### 6. 访问 Web 界面

打开浏览器访问：

```
http://localhost:1180
```

**默认管理员账号：**
- 用户名：`admin`
- 密码：`admin`

**⚠️ 重要**：首次登录后请立即修改默认密码！

### 7. 开始使用

1. 使用管理员账号登录
2. 在管理后台创建普通用户账号
3. 为用户设置初始余额和充值规则
4. 用户即可登录并开始打印

## 📖 详细使用指南

### 用户角色说明

#### 管理员（Admin）
- 管理所有用户账号
- 为用户充值
- 查看所有打印记录
- 配置系统设置（单页价格、数据保留等）
- 访问管理后台

#### 普通用户（User）
- 上传并打印文件
- 查看个人余额和消费记录
- 查看个人打印历史

### 余额和充值系统

#### 余额机制
- 每个用户有独立的打印余额账户（单位：分）
- 打印时根据页数和单页价格自动扣费
- 余额不足时无法打印

#### 自动充值
管理员可为用户配置自动充值规则：
- **每日充值**：每天自动充值指定金额
- **每月充值**：每月 1 号自动充值
- **每年充值**：每年 1 月 1 号自动充值

#### 手动充值
管理员可以随时为用户手动充值，系统会记录充值历史。

#### 消费限额
可为用户设置：
- **月度限额**：单月最大打印消费金额
- **年度限额**：单年最大打印消费金额

超过限额后将无法继续打印，直到下个周期或管理员调整限额。

### 打印功能

#### 支持的文件格式

| 格式类型 | 支持的扩展名 | 说明 |
|---------|------------|------|
| PDF | `.pdf` | 直接打印 |
| 图片 | `.jpg`, `.jpeg`, `.png`, `.gif` | 自动转换为 PDF |
| Office | `.docx`, `.xlsx`, `.pptx`, `.doc`, `.xls`, `.ppt` | 通过 LibreOffice 转换为 PDF |
| 文本 | `.txt` | 自动转换为 PDF |

#### 打印流程

1. **选择打印机**：从列表中选择可用的打印机
2. **上传文件**：点击选择文件按钮上传要打印的文件
3. **预览和转换**：
   - PDF 和图片可直接预览
   - Office 文档可点击"转换"按钮预览转换后的 PDF
4. **查看费用估算**：系统自动显示预估页数和费用
5. **确认打印**：点击"打印"按钮提交打印任务

#### 费用计算

```
打印费用 = 页数 × 单页价格
```

- 单页价格由管理员在系统设置中配置
- 默认单页价格：10 分（0.1 元）

#### 打印记录

用户可以查看自己的打印历史，包括：
- 打印时间
- 文件名
- 页数
- 费用
- 打印前后余额
- 打印状态

### 管理后台使用

#### 用户管理

**创建用户：**
1. 进入管理后台
2. 点击"创建用户"
3. 填写用户信息：
   - 用户名（必填）
   - 密码（必填）
   - 角色（管理员/普通用户）
   - 联系信息（可选）
   - 初始余额
   - 自动充值规则
   - 消费限额

**编辑用户：**
- 可修改用户的所有信息（除用户名外）
- 可调整余额、充值规则和限额

**删除用户：**
- 可删除普通用户
- 默认管理员账号（admin）受保护，无法删除

#### 充值管理

**手动充值：**
1. 在用户列表中找到目标用户
2. 点击"充值"按钮
3. 输入充值金额（单位：分）
4. 确认充值

**查看充值记录：**
- 可查看所有用户的充值历史
- 包括手动充值和自动充值记录
- 显示充值时间、金额、操作员等信息

#### 打印记录查询

管理员可以：
- 查看所有用户的打印记录
- 按用户名筛选
- 按时间范围筛选
- 导出打印记录（查看详细信息）

#### 系统设置

**单页价格：**
- 设置每页打印的费用（单位：分）
- 影响所有后续打印任务的费用计算

**数据保留天数：**
- 设置打印记录和上传文件的保留时间
- 超过保留期的数据会被自动清理

## ⚙️ 配置说明

### 环境变量详解

#### Web 服务配置

| 变量名 | 说明 | 默认值 | 必填 |
|--------|------|--------|------|
| `LISTEN_ADDR` | Web 服务监听地址 | `:8080` | 否 |
| `DB_PATH` | SQLite 数据库文件路径 | `/data/cups-web.db` | 否 |
| `UPLOAD_DIR` | 上传文件存储目录 | `/uploads` | 否 |
| `CUPS_HOST` | CUPS 服务地址 | `localhost` | 否 |
| `SESSION_HASH_KEY` | Session 加密哈希密钥 | - | **是** |
| `SESSION_BLOCK_KEY` | Session 加密块密钥 | - | **是** |
| `SESSION_SECURE` | 是否启用 HTTPS Cookie | `false` | 否 |

#### CUPS 服务配置

| 变量名 | 说明 | 默认值 | 必填 |
|--------|------|--------|------|
| `CUPSADMIN` | CUPS 管理员用户名 | - | **是** |
| `CUPSPASSWORD` | CUPS 管理员密码 | - | **是** |

### Docker Compose 配置

默认的 `docker-compose.yml` 配置：

- **CUPS 服务端口**：`631`（用于管理打印机）
- **Web 服务端口**：`1180`（用于访问 Web 界面）
- **数据持久化**：
  - `./.data`：数据库文件
  - `./.uploads`：上传的文件
  - `./.etc`：CUPS 配置文件

### 修改端口

如需修改端口，编辑 `docker-compose.yml`：

```yaml
services:
  web:
    ports:
      - "你的端口:8080"  # 修改左侧端口号
```

## 🔧 高级配置

### 使用 HTTPS

1. 在 `.env` 中设置：
```bash
SESSION_SECURE=true
```

2. 配置反向代理（如 Nginx）处理 HTTPS：

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:1180;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### 数据备份

定期备份以下目录：

```bash
# 备份数据库
cp ./.data/cups-web.db /backup/location/

# 备份上传文件
tar -czf uploads-backup.tar.gz ./.uploads/

# 备份 CUPS 配置
tar -czf cups-config-backup.tar.gz ./.etc/
```

### 性能优化

对于大量用户场景，建议：

1. 增加 Docker 容器资源限制
2. 定期清理过期的打印记录和文件
3. 使用 SSD 存储数据库文件

## ⚠️ 注意事项

### 安全建议

1. **修改默认密码**：首次部署后立即修改 admin 账号密码
2. **使用强密钥**：确保 `SESSION_HASH_KEY` 和 `SESSION_BLOCK_KEY` 足够随机和复杂
3. **启用 HTTPS**：生产环境建议使用 HTTPS 保护数据传输
4. **定期备份**：定期备份数据库和上传文件
5. **限制访问**：使用防火墙限制只有授权 IP 可以访问

### 打印机驱动

- CUPS 容器中可能没有预装所有打印机驱动
- 建议根据打印机型号手动安装对应驱动
- 可以通过 `docker exec` 进入 CUPS 容器安装驱动

### LibreOffice 转换

- Web 镜像已预装 LibreOffice 和常用字体
- 支持中文字体（Noto CJK、文泉驿等）
- 转换超时时间为 60 秒
- 复杂文档可能需要较长转换时间

### 数据清理

- 系统会根据"数据保留天数"设置自动清理过期数据
- 清理包括：打印记录和对应的上传文件
- 建议根据存储空间合理设置保留天数

## ❓ 常见问题

### 如何重置管理员密码？

如果忘记管理员密码，可以通过以下方式重置：

```bash
# 停止服务
docker-compose down

# 删除数据库（会清空所有数据）
rm ./.data/cups-web.db

# 重新启动服务（会创建新的 admin/admin 账号）
docker-compose up -d
```

### 打印机无法识别怎么办？

1. 确认打印机已正确连接到服务器
2. 访问 CUPS 管理界面（http://localhost:631）检查打印机状态
3. 尝试重启 CUPS 服务：`docker-compose restart cups`
4. 检查打印机驱动是否正确安装

### Office 文档转换失败？

可能的原因：
1. 文档格式损坏或不支持
2. 文档过大或过于复杂
3. LibreOffice 转换超时

解决方法：
1. 尝试在本地用 Office 或 LibreOffice 打开并另存为
2. 将文档手动转换为 PDF 后再上传
3. 简化文档内容

### 如何查看服务日志？

```bash
# 查看 Web 服务日志
docker-compose logs -f web

# 查看 CUPS 服务日志
docker-compose logs -f cups
```

### 如何更换打印机？

1. 访问 CUPS 管理界面（http://localhost:631）
2. 删除旧打印机
3. 添加新打印机
4. 在 Web 界面刷新打印机列表

## 📝 更新日志

查看 [Releases](https://github.com/hanxi/cups-web/releases) 了解版本更新历史。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

**编辑用户：**
- 可修改用户的所有信息（除用户名外）
- 可调整余额、充值规则和限额

**删除用户：**
- 可删除普通用户
- 默认管理员账号（admin）受保护，无法删除

#### 充值管理

**手动充值：**
1. 在用户列表中找到目标用户
2. 点击"充值"按钮
3. 输入充值金额（单位：分）
4. 确认充值

**查看充值记录：**
- 可查看所有用户的充值历史
- 包括手动充值和自动充值记录
- 显示充值时间、金额、操作员等信息

#### 打印记录查询

管理员可以：
- 查看所有用户的打印记录
- 按用户名筛选
- 按时间范围筛选
- 导出打印记录（查看详细信息）

#### 系统设置

**单页价格：**
- 设置每页打印的费用（单位：分）
- 影响所有后续打印任务的费用计算

**数据保留天数：**
- 设置打印记录和上传文件的保留时间
- 超过保留期的数据会被自动清理

## ⚙️ 配置说明

### 环境变量详解

#### Web 服务配置

| 变量名 | 说明 | 默认值 | 必填 |
|--------|------|--------|------|
| `LISTEN_ADDR` | Web 服务监听地址 | `:8080` | 否 |
| `DB_PATH` | SQLite 数据库文件路径 | `/data/cups-web.db` | 否 |
| `UPLOAD_DIR` | 上传文件存储目录 | `/uploads` | 否 |
| `CUPS_HOST` | CUPS 服务地址 | `localhost` | 否 |
| `SESSION_HASH_KEY` | Session 加密哈希密钥 | - | **是** |
| `SESSION_BLOCK_KEY` | Session 加密块密钥 | - | **是** |
| `SESSION_SECURE` | 是否启用 HTTPS Cookie | `false` | 否 |

#### CUPS 服务配置

| 变量名 | 说明 | 默认值 | 必填 |
|--------|------|--------|------|
| `CUPSADMIN` | CUPS 管理员用户名 | - | **是** |
| `CUPSPASSWORD` | CUPS 管理员密码 | - | **是** |

### Docker Compose 配置

默认的 `docker-compose.yml` 配置：

- **CUPS 服务端口**：`631`（用于管理打印机）
- **Web 服务端口**：`1180`（用于访问 Web 界面）
- **数据持久化**：
  - `./.data`：数据库文件
  - `./.uploads`：上传的文件
  - `./.etc`：CUPS 配置文件

### 修改端口

如需修改端口，编辑 `docker-compose.yml`：

```yaml
services:
  web:
    ports:
      - "你的端口:8080"  # 修改左侧端口号
```

## 🔧 高级配置

### 使用 HTTPS

1. 在 `.env` 中设置：
```bash
SESSION_SECURE=true
```

2. 配置反向代理（如 Nginx）处理 HTTPS：

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:1180;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### 数据备份

定期备份以下目录：

```bash
# 备份数据库
cp ./.data/cups-web.db /backup/location/

# 备份上传文件
tar -czf uploads-backup.tar.gz ./.uploads/

# 备份 CUPS 配置
tar -czf cups-config-backup.tar.gz ./.etc/
```

### 性能优化

对于大量用户场景，建议：

1. 增加 Docker 容器资源限制
2. 定期清理过期的打印记录和文件
3. 使用 SSD 存储数据库文件

## ⚠️ 注意事项

### 安全建议

1. **修改默认密码**：首次部署后立即修改 admin 账号密码
2. **使用强密钥**：确保 `SESSION_HASH_KEY` 和 `SESSION_BLOCK_KEY` 足够随机和复杂
3. **启用 HTTPS**：生产环境建议使用 HTTPS 保护数据传输
4. **定期备份**：定期备份数据库和上传文件
5. **限制访问**：使用防火墙限制只有授权 IP 可以访问

### 打印机驱动

- CUPS 容器中可能没有预装所有打印机驱动
- 建议根据打印机型号手动安装对应驱动
- 可以通过 `docker exec` 进入 CUPS 容器安装驱动

### LibreOffice 转换

- Web 镜像已预装 LibreOffice 和常用字体
- 支持中文字体（Noto CJK、文泉驿等）
- 转换超时时间为 60 秒
- 复杂文档可能需要较长转换时间

### 数据清理

- 系统会根据"数据保留天数"设置自动清理过期数据
- 清理包括：打印记录和对应的上传文件
- 建议根据存储空间合理设置保留天数

## ❓ 常见问题

### 如何重置管理员密码？

如果忘记管理员密码，可以通过以下方式重置：

```bash
# 停止服务
docker-compose down

# 删除数据库（会清空所有数据）
rm ./.data/cups-web.db

# 重新启动服务（会创建新的 admin/admin 账号）
docker-compose up -d
```

### 打印机无法识别怎么办？

1. 确认打印机已正确连接到服务器
2. 访问 CUPS 管理界面（http://localhost:631）检查打印机状态
3. 尝试重启 CUPS 服务：`docker-compose restart cups`
4. 检查打印机驱动是否正确安装

### Office 文档转换失败？

可能的原因：
1. 文档格式损坏或不支持
2. 文档过大或过于复杂
3. LibreOffice 转换超时

解决方法：
1. 尝试在本地用 Office 或 LibreOffice 打开并另存为
2. 将文档手动转换为 PDF 后再上传
3. 简化文档内容

### 如何查看服务日志？

```bash
# 查看 Web 服务日志
docker-compose logs -f web

# 查看 CUPS 服务日志
docker-compose logs -f cups
```

### 如何更换打印机？

1. 访问 CUPS 管理界面（http://localhost:631）
2. 删除旧打印机
3. 添加新打印机
4. 在 Web 界面刷新打印机列表

## 📝 更新日志

查看 [Releases](https://github.com/hanxi/cups-web/releases) 了解版本更新历史。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。
