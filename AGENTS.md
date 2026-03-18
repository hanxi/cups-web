# CUPS Web 项目指引

## 项目概述

CUPS Web 是一个功能完善的网页版打印机管理工具，允许通过浏览器远程控制打印机，支持多用户管理、打印记录追踪等功能。

**项目类型**: Web 应用程序  
**主要功能**: 远程打印、用户管理、打印记录追踪  
**技术栈**: Go + Vue.js 前后端分离架构

## 技术栈详情

### 后端技术栈
- **语言**: Go 1.24.0
- **Web框架**: Gorilla Mux
- **数据库**: SQLite (modernc.org/sqlite)
- **PDF处理**: gofpdf, rsc.io/pdf
- **打印机协议**: OpenPrinting/goipp (IPP协议)
- **会话管理**: Gorilla SecureCookie
- **加密**: golang.org/x/crypto

### 前端技术栈
- **框架**: Vue.js 3.5.26
- **构建工具**: Vite 7.3.0
- **样式框架**: Tailwind CSS 4.1.18 + DaisyUI 5.5.14
- **PDF处理**: jsPDF 3.0.4
- **包管理器**: Bun (推荐) / npm

## 项目结构

```
cups-web/
├── cmd/server/                 # 后端主程序
│   ├── assets/fonts/          # 字体资源
│   ├── admin_handlers.go      # 管理员接口
│   ├── auth_handlers.go       # 认证接口
│   ├── convert_handler.go     # 文件转换接口
│   ├── print_handlers.go      # 打印接口
│   └── main.go                # 程序入口
├── frontend/                  # 前端项目
│   ├── src/
│   │   ├── views/             # 页面组件
│   │   │   ├── LoginView.vue  # 登录页面
│   │   │   ├── PrintView.vue  # 打印页面
│   │   │   └── AdminView.vue  # 管理页面
│   │   ├── App.vue            # 根组件
│   │   └── main.js            # 入口文件
│   ├── package.json           # 前端依赖
│   └── vite.config.js         # Vite配置
├── internal/                  # 内部模块
│   ├── auth/                  # 认证模块
│   ├── ipp/                   # IPP协议客户端
│   ├── middleware/            # 中间件
│   └── store/                 # 数据存储
├── cups/                      # CUPS相关配置
├── screenshots/               # 界面截图
└── test/                      # 测试文件
```

## 核心模块说明

### 后端模块

1. **认证模块** (`internal/auth/`)
   - 会话管理
   - 用户认证

2. **存储模块** (`internal/store/`)
   - `users.go` - 用户管理
   - `prints.go` - 打印记录
   - `settings.go` - 系统设置

3. **IPP客户端** (`internal/ipp/`)
   - 打印机通信协议实现

### 前端模块

1. **登录页面** (`LoginView.vue`)
   - 用户认证界面

2. **打印页面** (`PrintView.vue`)
   - 文件上传和打印控制

3. **管理页面** (`AdminView.vue`)
   - 用户管理和系统设置

## 构建和部署

### 开发环境构建
```bash
# 构建前端 (需要Bun)
cd frontend && bun install && bun run build

# 构建后端
go build -o bin/cups-web ./cmd/server

# 或使用Makefile
make all
```

### Docker部署
```bash
# 构建镜像
docker build -t cups-web:latest -f Dockerfile .

# 使用docker-compose
docker-compose up -d
```

## 环境变量配置

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `LISTEN_ADDR` | `:8080` | 服务监听地址 |
| `DB_PATH` | `data/cups-web.db` | 数据库文件路径 |
| `UPLOAD_DIR` | `uploads` | 文件上传目录 |

## 数据库结构

项目使用 SQLite 数据库，主要表结构：
- `users` - 用户信息表
- `prints` - 打印记录表
- `settings` - 系统设置表

## API接口规范

### 认证接口
- `POST /api/login` - 用户登录
- `POST /api/logout` - 用户登出

### 打印接口
- `GET /api/printers` - 获取打印机列表
- `POST /api/print` - 提交打印任务
- `GET /api/prints` - 获取打印记录

### 管理接口
- `GET /api/users` - 获取用户列表
- `POST /api/users` - 创建用户

## 代码规范

### Go代码规范
- 使用标准Go命名约定
- 错误处理使用多返回值模式
- 接口定义清晰，模块化设计

### Vue.js代码规范
- 使用Composition API
- 组件采用单文件组件(SFC)格式
- 样式使用Tailwind CSS类名

## 文件处理流程

1. **文件上传**: 用户上传文件到服务器
2. **格式转换**: 将Office文档、图片等转换为PDF
3. **PDF处理**: 使用LibreOffice进行格式转换
4. **打印提交**: 通过IPP协议发送到打印机
5. **记录保存**: 保存打印记录

## 安全考虑

1. **会话安全**: 使用SecureCookie进行会话管理
2. **文件安全**: 上传文件类型检查和路径安全
3. **权限控制**: 基于角色的访问控制(RBAC)
4. **CSRF防护**: 内置CSRF中间件保护

## 扩展性设计

### 后端扩展
- 模块化设计，易于添加新功能
- 接口清晰，便于集成新打印机协议
- 数据库抽象层支持多种存储后端

### 前端扩展
- 组件化架构，易于添加新页面
- 响应式设计，支持多设备访问
- 国际化支持准备

## 常见开发任务

### 添加新API接口
1. 在`cmd/server/`下创建对应的handler文件
2. 在`main.go`中注册路由
3. 更新前端Vue组件调用新接口

### 修改数据库结构
1. 更新`internal/store/`中的对应模型
2. 执行数据库迁移
3. 更新相关业务逻辑

### 添加新前端页面
1. 在`frontend/src/views/`创建Vue组件
2. 在`App.vue`中注册路由
3. 更新导航菜单

## 调试和测试

### 后端调试
- 使用标准Go调试工具
- 日志输出到控制台
- 数据库文件可本地查看

### 前端调试
- 使用Vite开发服务器
- 支持热重载开发
- 浏览器开发者工具

## 性能优化建议

1. **前端优化**: 代码分割、懒加载
2. **后端优化**: 连接池、缓存策略
3. **数据库优化**: 索引优化、查询优化

---

## 🔧 开发环境搭建

### 本地开发

```bash
# 1. 克隆项目
git clone <repo-url>
cd cups-web

# 2. 构建前端
cd frontend
bun install
bun run dev  # 开发模式
bun run build  # 生产构建

# 3. 构建后端
cd ..
go mod download
go build -o bin/cups-web ./cmd/server

# 4. 运行服务
./bin/cups-web
```

### 使用 Makefile

```bash
# 查看可用命令
make help

# 构建所有
make all

# 仅构建后端
make build

# 仅构建前端
make frontend

# 清理构建
make clean
```

## 📁 完整项目结构

```
cups-web/
├── cmd/server/                 # 后端主程序
│   ├── assets/fonts/          # 字体资源
│   ├── admin_handlers.go      # 管理员接口处理器
│   ├── auth_handlers.go       # 认证接口处理器
│   ├── bootstrap.go           # 应用初始化
│   ├── convert_handler.go     # 文件转换接口
│   ├── convert_utils.go       # 转换工具函数
│   ├── estimate_handler.go    # 页数估算接口
│   ├── file_utils.go          # 文件处理工具
│   ├── fonts.go               # 字体管理
│   ├── main.go                # 程序入口
│   ├── maintenance.go         # 维护模式处理
│   ├── pdf_utils.go           # PDF 处理工具
│   ├── print_handlers.go      # 打印接口处理器
│   ├── print_records_handlers.go  # 打印记录接口
│   ├── printer_info_handler.go    # 打印机信息接口
│   └── user_handlers.go       # 用户管理接口
├── frontend/                  # 前端项目
│   ├── src/
│   │   ├── views/             # 页面组件
│   │   │   ├── LoginView.vue  # 登录页面
│   │   │   ├── PrintView.vue  # 打印页面
│   │   │   └── AdminView.vue  # 管理页面
│   │   ├── App.vue            # 根组件
│   │   ├── main.js            # 入口文件
│   │   └── index.css          # 全局样式
│   ├── package.json           # 前端依赖
│   └── vite.config.js         # Vite 配置
├── internal/                  # 内部模块
│   ├── auth/                  # 认证模块
│   ├── ipp/                   # IPP 协议客户端
│   ├── middleware/            # 中间件
│   └── store/                 # 数据存储
│       ├── prints.go          # 打印记录存储
│       ├── settings.go        # 系统设置存储
│       └── users.go           # 用户存储
├── cups/                      # CUPS 相关配置
├── screenshots/               # 界面截图
├── test/                      # 测试文件
├── bin/                       # 编译输出
├── data/                      # 数据库文件
├── uploads/                   # 上传文件
├── .aone_copilot/             # AI 助手计划
├── .github/                   # GitHub 配置
├── docker-compose.yml         # Docker Compose 配置
├── Dockerfile                 # Docker 镜像构建
├── go.mod                     # Go 模块定义
├── go.sum                     # Go 依赖锁定
├── LICENSE                    # 许可证
├── Makefile                   # 构建脚本
├── README.md                  # 用户文档
└── AGENTS.md                  # 开发者文档
```

## 🎯 关键开发任务指南

### 添加新的 API 端点

1. **创建 Handler**
   - 在 `cmd/server/` 下创建对应的 handler 文件（如 `xxx_handler.go`）
   - 实现处理函数，遵循现有代码模式
   - 添加适当的错误处理和日志记录

2. **注册路由**
   - 在 `main.go` 中找到路由注册部分
   - 添加新的路由映射
   - 应用适当的中间件（如认证、CSRF）

3. **更新前端**
   - 在对应的 Vue 组件中添加 API 调用
   - 更新状态管理和 UI 展示

### 修改数据库结构

1. **更新模型定义**
   - 在 `internal/store/` 中找到对应的模型文件
   - 添加新字段或新表
   - 更新相关的 CRUD 操作

2. **执行迁移**
   - 编写数据库迁移脚本
   - 确保向后兼容
   - 测试迁移过程

3. **更新业务逻辑**
   - 更新所有使用到该模型的代码
   - 确保数据一致性

### 添加新的前端页面

1. **创建 Vue 组件**
   - 在 `frontend/src/views/` 下创建新的 `.vue` 文件
   - 使用 Composition API 组织逻辑
   - 应用 Tailwind CSS + DaisyUI 样式

2. **注册路由**
   - 在 `App.vue` 中添加路由配置
   - 设置适当的权限控制

3. **更新导航**
   - 在导航菜单中添加新页面的入口
   - 根据用户角色显示/隐藏菜单项

## 🧪 测试指南

### 后端测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./cmd/server/...

# 带覆盖率报告
go test -cover ./...
```

### 前端测试

```bash
cd frontend

# 运行测试
bun run test

# 构建检查
bun run build
```

## 📊 监控和日志

### 日志查看

```bash
# Docker 环境
docker-compose logs -f web
docker-compose logs -f cups

# 本地开发
# 日志输出到控制台
```

### 数据库检查

```bash
# 查看 SQLite 数据库
sqlite3 data/cups-web.db

# 查看表结构
.tables

# 查询数据
SELECT * FROM users;
SELECT * FROM prints LIMIT 10;
```

## 🔐 安全检查清单

- [ ] 所有敏感配置使用环境变量
- [ ] Session 密钥足够随机和复杂
- [ ] 文件上传进行类型和大小限制
- [ ] SQL 查询使用参数化防止注入
- [ ] CSRF 保护已启用
- [ ] 密码使用 bcrypt 加密
- [ ] 生产环境启用 HTTPS

## 📚 相关资源

- [CUPS 官方文档](https://www.cups.org/)
- [Go 语言文档](https://golang.org/doc/)
- [Vue.js 文档](https://vuejs.org/)
- [Tailwind CSS](https://tailwindcss.com/)
- [DaisyUI](https://daisyui.com/)

---

**最后更新**: 2026-03-18  
**维护者**: 涵曦 (im.hanxi@gmail.com)  
**文档版本**: v1.1

## 📚 文档说明

本文档 (`AGENTS.md`) 主要面向开发者，提供详细的技术架构、开发指南和内部实现细节。用户文档请参阅 `README.md`。

### 文档分工

- **README.md**: 用户快速开始指南、功能介绍、部署说明
- **AGENTS.md**: 开发者技术文档、架构说明、API 规范、开发流程