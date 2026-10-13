# Web Modem 调测工具

[![Go Version](https://img.shields.io/badge/Go-1.26.3+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Web-Modem** 是一款专为物联网开发人员设计的 Modem 模块调试与测试工具，提供直观的 Web 界面，帮助开发者快速连接、配置和调测各类 Modem 设备。

## ⚠️ 特别声明

本工具仅限用于物联网设备开发调测，禁止用于任何非法场景。

## ✨ 功能特性

- **设备管理**：自动扫描、多设备支持、实时状态监控、AT 指令调测
- **短信功能**：PDU 模式收发、Unicode 编码、数据库存储、批量管理
- **Webhook 通知**：实时推送、自定义模板、批量触发、重试机制
- **数据可视化**：SSE 实时推送、分页筛选、设备信息展示
- **eSIM 管理**：LPA/APDU、Profile 列表与下载、启用/禁用/删除、号码登记、轮询收信
- **SIM 与网络**：PIN/PUK、USSD/STK、APDU、电话簿、PDP、运营商选择、蜂窝/本机 ping
- **高级功能**：数据同步、自更新、跨平台支持、Basic Auth 身份认证

## 🚀 快速开始

### 环境要求

- **操作系统**：Linux / Windows / macOS
- **硬件**：支持标准 AT 指令集的 Modem 设备
- **源码构建**：Go 1.26.3+、Node.js 22、Corepack（pnpm 9.15.9）
- **权限**：访问串口设备的权限（Linux 下需要 dialout 组）

### 安装运行

Linux / macOS 推荐使用一键安装脚本，脚本会自动下载最新版本、安装二进制，并在 Linux systemd 或 macOS launchd 环境下注册服务：

```bash
curl -fsSL https://raw.githubusercontent.com/rehiy/web-modem/master/install.sh | sh
```

安装完成后访问 `http://localhost:8080` 打开 Web 界面。首次安装默认**不启用鉴权**（无密码直连），需要鉴权时再通过环境变量开启。

可通过环境变量调整安装参数：

```bash
# 启用 Basic Auth（自动生成随机密码并输出到终端）
AUTH_MODE=on sh install.sh
# 启用 Basic Auth 并指定账号密码
BASIC_AUTH_USER=admin BASIC_AUTH_PASSWORD=your-password sh install.sh
# 安装到用户目录，无需 sudo
HTTP_PORT=18080 INSTALL_SERVICE=0 INSTALL_DIR="$HOME/.local/bin" \
  ENV_FILE="$HOME/.config/web-modem/modem.env" DB_PATH="$HOME/.local/share/web-modem/modem.db" sh install.sh
```

> 说明：`AUTH_MODE=auto`（默认）时，只要提供了 `BASIC_AUTH_PASSWORD` 就启用鉴权；`AUTH_MODE=on` 强制启用；`AUTH_MODE=off` 强制无鉴权（会写入不含 `BASIC_AUTH_*` 的配置文件）。
> 配置文件已存在时安装脚本不会覆盖，如需切换鉴权模式，请先备份并删除配置文件（Linux: `/etc/default/web-modem`，macOS: `/usr/local/etc/web-modem.env`）后重新安装。

也可以访问 [Releases](https://github.com/rehiy/web-modem/releases) 手动下载对应系统的二进制文件：

```bash
# 配置认证（可选）
export BASIC_AUTH_USER=admin
export BASIC_AUTH_PASSWORD=password
# 启动服务
chmod +x web-modem && ./web-modem
```

### 源码编译

```bash
# 克隆项目
git clone https://github.com/rehiy/web-modem.git
cd web-modem

# 安装依赖
go mod download

# 编译运行
(cd webui && corepack pnpm install --frozen-lockfile && corepack pnpm build)
go build -o web-modem .
./web-modem
```

前端源码位于 `webui/`，使用 Vite + React + TypeScript + Tailwind；构建产物输出到 `webview/dist/` 并由 Go embed 打包进二进制。

开发调试可运行 `./start-dev.sh`，同时启动 Go API 与 Vite。发布时运行 `sh build.sh`，产出各平台 gzip 二进制和 `checksums.txt`；GitHub Actions 在推送版本 tag 后发布到本仓库 Releases。模块路径保留 `github.com/rehiy/web-modem`，依赖使用上游 `github.com/rehiy/modem v0.0.6`（包含单命令超时、SIM 状态和自定义 URC 支持）及 `github.com/rehiy/libgo`。

自更新默认读取 `rehiy/web-modem` 的 GitHub Releases，校验 GitHub 资产的 SHA256 digest（旧版资产无 digest 时读取 `checksums.txt`），再替换二进制并按需重启。源码构建可用 `UPDATE_REPO=owner/repo sh build.sh` 指定 GitHub 更新仓库；安装脚本可用 `GITHUB_REPO=owner/repo sh install.sh` 指定下载仓库，公开仓库无需 Token。

### 配置选项

| 变量名 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `DB_PATH` | 数据库文件路径 | `data/modem.db` |
| `HTTP_PORT` | HTTP 监听端口 | `8080` |
| `MODEM_PORT` | 串口设备，多个用逗号分隔 | Linux: /dev/ttyUSB*,/dev/ttyACM*; Windows: COM1-COM5 |
| `MODEM_USB_IDS` | Linux 下自动绑定到 option 串口驱动的 USB VID/PID，多个用逗号分隔 | `2ecc:3012` |
| `MODEM_DISABLE_IFACE_MACS` | Linux 下自动关闭的冲突网卡 MAC，多个用逗号分隔；设为空格可禁用 | `ac:0c:29:a3:9b:6d` |
| `IP_MONITOR_IGNORE_MACS` | IP 变化通知忽略的网卡 MAC，多个用逗号分隔；未设置时跟随 `MODEM_DISABLE_IFACE_MACS` | `ac:0c:29:a3:9b:6d` |
| `BASIC_AUTH_USER` | Basic Auth 用户名 | 无（不启用） |
| `BASIC_AUTH_PASSWORD` | Basic Auth 密码 | 无（不启用） |
| `AUTH_MODE` | 安装脚本鉴权模式：`auto`/变量推断、`on` 强制鉴权、`off` 无鉴权 | `auto` |

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
- 短信库自动存储默认启用，Webhook 默认关闭，可在设置中独立切换；关闭短信库存储后仍会清理设备短信，需要保留短信时请开启存储

### 3. eSIM 管理

- 选择支持 eUICC 的设备，在 eSIM 页面查看 Profile 和卡片信息
- 输入或扫描 LPA 激活码下载 Profile，可启用、禁用或删除 Profile
- 为 Profile 登记接收号码，便于短信库按号码与 ICCID 归档
- 按设备启用轮询并设置轮询间隔，让多个 Profile 轮流上线收信；切卡期间等待 SIM 就绪和设备重连

### 4. Webhook 配置

- 切换到 "Webhook" 标签页
- 添加 Webhook，填写名称、URL 和模板
- 预设模板中可选择 "飞书卡片" 直接发送飞书 interactive 卡片（内容支持 `{{占位符}}` 替换）
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
POST /api/smsdb/sync           # 同步短信；不传 name 时同步所有已连接卡
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

### 自更新 API

```http
GET  /api/update/check         # 检查最新版本
POST /api/update/apply         # 下载并应用更新
POST /api/update/restart       # 重启应用
```

### eSIM API

```http
GET    /api/euicc/profiles?name=xxx              # Profile 列表
POST   /api/euicc/download                      # 下载 Profile
POST   /api/euicc/profiles/{iccid}/enable        # 启用
POST   /api/euicc/profiles/{iccid}/disable       # 禁用
DELETE /api/euicc/profiles/{iccid}               # 删除
PUT    /api/euicc/profiles/{iccid}/number        # 登记号码
GET    /api/euicc/rotation?name=xxx              # 轮询配置
PUT    /api/euicc/rotation                      # 保存轮询配置
POST   /api/euicc/rotation/run                  # 手动执行轮询
```

### SSE API

```http
GET /events/modem              # Modem 事件实时推送
```

## ⚠️ 注意事项

### 合法使用

本工具仅限用于物联网设备开发、测试和调试场景，严禁用于：

- 发送垃圾短信或骚扰信息
- 非法监控或窃取他人信息
- 任何违反法律法规的行为

### 安全建议

1. 默认安装为无鉴权模式，暴露到公网前请务必通过 `AUTH_MODE=on` 安装或手动配置 `BASIC_AUTH_USER`/`BASIC_AUTH_PASSWORD` 启用认证
2. 使用 HTTPS 保护数据传输安全
3. 确保有足够的串口访问权限
4. 定期备份数据库

### 常见问题

**Q: Linux 下扫描不到设备？**

某些 USB Modem 设备可能需要加载驱动补丁才能被识别

```bash
# 程序启动和扫描设备时会自动执行等价补丁；也可以手动运行
# 修改设备 ID 和 MAC 以匹配实际设备
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
