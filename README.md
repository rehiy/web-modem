# Web Modem 调测工具

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Web-Modem** 是一款专为物联网开发人员设计的 Modem 模块调试与测试工具，提供直观的 Web 界面，帮助开发者快速连接、配置和调测各类 Modem 设备。

## ⚠️ 特别声明

本工具仅限用于物联网设备开发调测，禁止用于任何非法场景。

## ✨ 功能特性

- **设备管理**：自动扫描、多设备支持、实时状态监控、AT 指令调测
- **短信功能**：PDU 模式收发、Unicode 编码、数据库存储、批量管理
- **Webhook 通知**：实时推送、自定义模板、批量触发、重试机制
- **数据可视化**：WebSocket 实时推送、分页筛选、设备信息展示
- **高级功能**：数据同步、跨平台支持、Basic Auth 身份认证

## 🚀 快速开始

### 环境要求

- **操作系统**：Linux / Windows / macOS
- **硬件**：支持标准 AT 指令集的 Modem 设备
- **权限**：访问串口设备的权限（Linux 下需要 dialout 组）

### 安装运行

访问 [Releases](https://github.com/rehiy/web-modem/releases) 下载对应系统的二进制文件：

```bash
# 配置认证（可选）
export BASIC_AUTH_USER=admin
export BASIC_AUTH_PASSWORD=password
# 启动服务
chmod +x web-modem && ./web-modem
```

访问 `http://localhost:8080` 打开 Web 界面。

### 源码编译

```bash
# 克隆项目
git clone https://github.com/rehiy/web-modem.git
cd web-modem

# 安装依赖
go mod download

# 编译运行
go build -o web-modem .
./web-modem
```

### 配置选项

| 变量名 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `DB_PATH` | 数据库文件路径 | `data/modem.db` |
| `HTTP_PORT` | HTTP 监听端口 | `8080` |
| `MODEM_PORT` | 串口设备，多个用逗号分隔 | Linux: /dev/ttyUSB*,/dev/ttyACM*; Windows: COM1-COM5 |
| `BASIC_AUTH_USER` | Basic Auth 用户名 | 无（不启用） |
| `BASIC_AUTH_PASSWORD` | Basic Auth 密码 | 无（不启用） |

## 📖 使用指南

### 1. 设备管理

- 将 Modem 通过 USB 连接计算机
- 点击 "扫描设备" 自动检测可用串口
- 选择设备查看信息（制造商、IMEI、信号强度等）
- 发送 AT 指令调测（如 `AT+CGMI`）

### 2. 短信收发

- 切换到 "短信" 标签页
- 选择设备，输入号码和内容发送短信
- 自动接收 incoming 短信，支持 Unicode 中文
- 收到短信后自动删除设备上的短信（数据库中保留）

### 3. Webhook 配置

- 切换到 "Webhook" 标签页
- 添加 Webhook，填写名称、URL 和模板
- 默认禁用，需在设置中启用后才会触发
- 点击 "测试" 验证配置

## 🔌 API 文档

### Modem API

```http
GET  /api/modem/list          # 扫描设备
POST /api/modem/send          # 发送 AT 指令
GET  /api/modem/info?name=xxx # 获取设备信息
GET  /api/modem/signal?name=xxx # 获取信号强度
GET  /api/modem/sms/list?name=xxx # 获取短信列表
POST /api/modem/sms/send      # 发送短信
POST /api/modem/sms/delete    # 删除短信
```

### 数据库 API

```http
GET  /api/smsdb/list?direction=in&limit=50&offset=0 # 查询短信（支持分页）
POST /api/smsdb/delete         # 批量删除
POST /api/smsdb/sync           # 同步短信
```

### Webhook API

```http
POST   /api/webhook           # 创建 Webhook
GET    /api/webhook/list       # 获取列表
GET    /api/webhook/get?id=1   # 获取单个 Webhook
PUT    /api/webhook/update?id=1 # 更新
DELETE /api/webhook/delete?id=1 # 删除
POST   /api/webhook/test?id=1  # 测试
```

### 设置 API

```http
GET /api/settings              # 获取所有设置
PUT /api/settings/smsdb        # 更新短信存储设置
PUT /api/settings/webhook      # 更新 Webhook 设置
```

### WebSocket API

```http
WS /ws/modem                   # Modem 事件实时推送
```

## ⚠️ 注意事项

### 合法使用

本工具仅限用于物联网设备开发、测试和调试场景，严禁用于：

- 发送垃圾短信或骚扰信息
- 非法监控或窃取他人信息
- 任何违反法律法规的行为

### 安全建议

1. 生产环境建议启用 Basic Auth 认证
2. 使用 HTTPS 保护数据传输安全
3. 确保有足够的串口访问权限
4. 定期备份数据库

### 常见问题

**Q: Linux 下扫描不到设备？**

某些 USB Modem 设备可能需要加载驱动补丁才能被识别

```bash
# 修改设备 ID 和 MAC 以匹配实际设备，然后再运行
chmod +x patch.sh && ./patch.sh
# Linux 查看串口
ls -la /dev/ttyUSB*
# Windows 查看 COM 端口
mode
```

**Q: Linux 下无法访问串口？**

添加用户到 dialout 组，然后重启

```bash
sudo usermod -a -G dialout $USER
```

**Q: Webhook 触发失败？**

检查目标 URL 是否可达，查看日志，确认功能已启用。

## 📄 许可证

GPL-3.0 许可证 - 详见 [LICENSE](LICENSE)
