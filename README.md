# Web Modem 调测工具

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows%20%7C%20macOS-lightgrey.svg)](https://github.com/rehiy/web-modem)

**一款专为物联网开发设计的 Modem 模块调测可视化工具**

⚠️ **特别声明：本工具仅限用于物联网设备开发调测，禁止用于任何非法场景**

[功能特性](#功能特性) | [快速开始](#快速开始) | [使用指南](#使用指南) | [API文档](#api文档) | [配置说明](#配置说明)

---

## 📖 项目简介

Web Modem 调测工具是一款专为物联网开发人员设计的 Modem 模块调试与测试工具。它提供了直观的 Web 界面，帮助开发者快速连接、配置和调测各类 Modem 设备，实现短信收发、设备管理、数据存储等功能。

本项目基于 Go 语言开发，采用前后端分离架构，支持多平台运行，是物联网设备开发过程中不可或缺的调测助手。

## ✨ 功能特性

### 🔌 模块管理

- **自动扫描**：自动检测并连接系统中可用的串口设备
- **多设备支持**：同时管理多个 Modem 设备
- **状态监控**：实时显示设备连接状态、信号强度等信息
- **AT 指令调测**：支持发送自定义 AT 指令，查看响应结果

### 💬 短信功能

- **发送短信**：通过 PDU 模式发送短信，支持 Unicode 编码
- **接收短信**：实时接收并显示 incoming 短信
- **短信管理**：支持短信的查看、删除、批量操作
- **数据库存储**：自动将短信保存到 SQLite 数据库

### 🔔 Webhook 通知

- **实时推送**：接收到短信时自动触发 Webhook
- **自定义模板**：支持自定义 Webhook 请求模板
- **批量触发**：可同时配置多个 Webhook 接收端
- **重试机制**：失败自动重试，支持指数退避算法

### 📊 数据可视化

- **实时日志**：WebSocket 实时推送设备状态和调试信息
- **短信列表**：分页显示、条件筛选短信记录
- **设备信息**：直观展示 Modem 基本信息和信号状态

### 🔧 高级功能

- **数据同步**：将 Modem 中的短信同步到数据库
- **缓存优化**：Webhook 列表缓存，提升性能
- **跨平台**：支持 Linux、Windows、macOS 系统

## 🏗️ 技术架构

```text
┌─────────────────────────────────────────────────────────────┐
│                        Web 界面层                            │
│              HTML + CSS + JavaScript (原生)                  │
└──────────────────────────┬──────────────────────────────────┘
                           │ HTTP / WebSocket
┌──────────────────────────▼──────────────────────────────────┐
│                      API 接口层 (Handler)                    │
│  ModemHandler  SmsdbHandler  WebhookHandler  SettingHandler │
└──────────────────────────┬──────────────────────────────────┘
                           │ 业务调用
┌──────────────────────────▼──────────────────────────────────┐
│                      业务逻辑层 (Service)                    │
│    ModemService    WebhookService    SmsdbService          │
└──────────────────────────┬──────────────────────────────────┘
                           │ 数据访问
┌──────────────────────────▼──────────────────────────────────┐
│                      数据访问层 (Database)                   │
│    SQLite + GORM    支持 SMS/Webhook/Setting 数据表         │
└──────────────────────────┬──────────────────────────────────┘
                           │ AT 指令
┌──────────────────────────▼──────────────────────────────────┐
│                      设备通信层                              │
│              串口通信 + AT 指令集 (PDU模式)                  │
└─────────────────────────────────────────────────────────────┘
```

## 🚀 快速开始

### 环境要求

- **操作系统**：Linux / Windows / macOS
- **Go 版本**：1.21 或更高
- **硬件**：支持标准 AT 指令集的 Modem 设备
- **权限**：访问串口设备的权限（Linux 下通常需要 dialout 组）

### 安装步骤

1. **克隆项目**

```bash
git clone https://github.com/rehiy/web-modem.git
cd web-modem
```

1. **安装依赖**

```bash
go mod download
```

1. **编译项目**

```bash
go build -o web-modem .
```

1. **运行程序**

```bash
# 直接运行（自动扫描串口）
./web-modem

# 指定串口设备（Linux）
MODEM_PORT=/dev/ttyUSB0, /dev/ttyUSB1 ./web-modem

# 指定串口设备（Windows）
MODEM_PORT=COM1,COM2 ./web-modem

# 指定数据库路径
DB_PATH=/custom/path/modem.db ./web-modem
```

1. **访问界面**
打开浏览器访问：`http://localhost:8080`

## 📖 使用指南

### 1. Modem 设备管理

**连接设备**

1. 将 Modem 设备通过 USB 连接到计算机
2. 点击主界面的 "扫描设备" 按钮
3. 系统会自动检测并连接可用的串口设备

**查看设备信息**

- 在设备列表中点击具体设备
- 查看制造商、型号、IMEI、IMSI、运营商等信息
- 实时信号强度显示（dBm）

**发送 AT 指令**

- 在 "AT 指令" 输入框中输入指令（如 `AT+CGMI`）
- 点击发送查看响应结果
- 支持多条指令连续发送

### 2. 短信收发

**发送短信**

1. 切换到 "短信" 标签页
2. 选择目标 Modem 设备
3. 输入接收号码和短信内容
4. 点击发送

**接收短信**

- 系统自动接收 incoming 短信
- 新短信实时显示在列表中
- 支持 Unicode 中文短信

**短信存储**

- 启用 "数据库存储" 功能
- 所有短信自动保存到 SQLite 数据库
- 支持分页查看、条件筛选

### 3. Webhook 配置

**添加 Webhook**

1. 切换到 "Webhook" 标签页
2. 点击 "添加 Webhook"
3. 填写名称、URL 和请求模板（可选）
4. 保存配置

**启用 Webhook**

- 勾选 "启用 Webhook 功能"
- 接收到短信时自动触发
- 支持多个 Webhook 同时触发

**测试 Webhook**

- 点击 Webhook 列表中的 "测试" 按钮
- 发送测试短信到配置的 URL
- 查看响应结果和状态

## 🔌 API 文档

### Modem 管理 API

#### 扫描设备

```http
GET /api/modem/list
```

#### 发送 AT 指令

```http
POST /api/modem/send
Content-Type: application/json

{
  "name": "ttyUSB0",
  "command": "AT+CGMI"
}
```

#### 获取设备信息

```http
GET /api/modem/info?name=ttyUSB0
```

#### 获取信号强度

```http
GET /api/modem/signal?name=ttyUSB0
```

### 短信 API

#### 获取短信列表

```http
GET /api/modem/sms/list?name=ttyUSB0
```

#### 发送短信

```http
POST /api/modem/sms/send
Content-Type: application/json

{
  "name": "ttyUSB0",
  "number": "+8613800138000",
  "message": "Hello, World!"
}
```

#### 删除短信

```http
POST /api/modem/sms/delete
Content-Type: application/json

{
  "name": "ttyUSB0",
  "indices": [0, 1, 2]
}
```

### 数据库短信 API

#### 查询短信列表

```http
GET /api/smsdb/list?direction=in&limit=50&offset=0
```

#### 批量删除短信

```http
POST /api/smsdb/delete
Content-Type: application/json

{
  "ids": [1, 2, 3]
}
```

#### 同步短信

```http
POST /api/smsdb/sync
Content-Type: application/json

{
  "name": "ttyUSB0"
}
```

### Webhook API

#### 创建 Webhook

```http
POST /api/webhook
Content-Type: application/json

{
  "name": "My Webhook",
  "url": "https://example.com/webhook",
  "template": "{\"content\": \"{{content}}\"}",
  "enabled": true
}
```

#### 获取 Webhook 列表

```http
GET /api/webhook/list
```

#### 更新 Webhook

```http
PUT /api/webhook/update?id=1
Content-Type: application/json

{
  "name": "Updated Webhook",
  "url": "https://example.com/new-webhook"
}
```

#### 删除 Webhook

```http
DELETE /api/webhook/delete?id=1
```

#### 测试 Webhook

```http
POST /api/webhook/test?id=1
```

### 设置 API

#### 获取所有设置

```http
GET /api/settings
```

#### 更新短信存储设置

```http
PUT /api/settings/smsdb
Content-Type: application/json

{
  "smsdb_enabled": true
}
```

#### 更新 Webhook 设置

```http
PUT /api/settings/webhook
Content-Type: application/json

{
  "webhook_enabled": true
}
```

## ⚙️ 配置说明

### 环境变量

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `MODEM_PORT` | 指定串口设备，多个用逗号分隔 | 自动扫描 | `/dev/ttyUSB0,COM1` |
| `DB_PATH` | 数据库文件路径 | `./data/modem.db` | `/var/lib/modem.db` |
| `HTTP_ADDR` | HTTP 监听地址 | `:8080` | `:80` |
| `LOG_LEVEL` | 日志级别 | `info` | `debug`, `warn`, `error` |

### 配置文件示例

```bash
# Linux 系统
export MODEM_PORT=/dev/ttyUSB0,/dev/ttyUSB1
export DB_PATH=/var/lib/web-modem/modem.db
export HTTP_ADDR=:8080
./web-modem

# Windows 系统
set MODEM_PORT=COM1,COM2
set DB_PATH=C:\web-modem\data\modem.db
set HTTP_ADDR=:8080
web-modem.exe
```

## 🗄️ 数据库结构

项目使用 SQLite 数据库，自动创建以下数据表：

### SMS 表（短信记录）

```sql
CREATE TABLE sms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    sms_ids TEXT NOT NULL,
    receive_time DATETIME NOT NULL,
    receive_number TEXT,
    send_number TEXT,
    direction TEXT NOT NULL CHECK(direction IN ('in', 'out')),
    modem_name TEXT,
    created_at DATETIME
);
```

### Webhook 表（Webhook配置）

```sql
CREATE TABLE webhooks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    template TEXT DEFAULT '{}',
    enabled BOOLEAN DEFAULT true,
    created_at DATETIME,
    updated_at DATETIME
);
```

### Setting 表（系统设置）

```sql
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    created_at DATETIME,
    updated_at DATETIME
);
```

## ⚠️ 注意事项

### 合法使用声明

**本工具仅限用于物联网设备开发、测试和调试场景，严禁用于以下用途：**

- ❌ 发送垃圾短信或骚扰信息
- ❌ 非法监控或窃取他人信息
- ❌ 任何违反法律法规的行为
- ❌ 任何侵犯他人隐私的行为

使用者应当：

- 遵守所在国家和地区的法律法规
- 仅在拥有合法授权的设备和网络上使用
- 尊重他人隐私和通信自由

### 安全建议

1. **访问控制**：在生产环境部署时，建议添加身份认证机制
2. **网络安全**：使用 HTTPS 协议保护数据传输安全
3. **设备权限**：确保有足够的串口访问权限（Linux 下建议加入 dialout 组）
4. **数据备份**：定期备份数据库，防止数据丢失

### 常见问题

**Q: Linux 下无法访问串口设备？**
A: 将用户添加到 dialout 组：

```bash
sudo usermod -a -G dialout $USER
# 注销后重新登录生效
```

**Q: 扫描不到 Modem 设备？**
A: 检查设备连接和驱动：

```bash
# Linux 查看串口设备
ls -la /dev/ttyUSB*
# Windows 查看 COM 端口
mode
```

**Q: Webhook 触发失败？**
A: 检查目标 URL 是否可达，查看日志中的错误信息，确认 Webhook 功能已启用

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request 来帮助改进项目！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 📄 许可证

本项目采用 GPL-3.0 许可证 - 详见 [LICENSE](LICENSE) 文件

## 🙏 致谢

- [gorilla/mux](https://github.com/gorilla/mux) - HTTP 路由库
- [glebarez/sqlite](https://github.com/glebarez/sqlite) - SQLite 驱动
- [tarm/serial](https://github.com/tarm/serial) - 串口通信库
- [modem/at](https://github.com/rehiy/modem) - AT 指令库

---

**⚠️ 再次声明：本工具仅限用于物联网设备开发调测，请合法合规使用！**

如果您在使用过程中遇到任何问题，请通过 GitHub Issues 反馈。
