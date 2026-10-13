#!/usr/bin/env bash
#
# Web Modem 开发环境一键启动脚本
# 用法:
#   ./start-dev.sh
#   BACKEND_PORT=8080 FRONTEND_PORT=5173 ./start-dev.sh
#   BASIC_AUTH_USER=admin BASIC_AUTH_PASSWORD=password ./start-dev.sh
#

set -euo pipefail

BACKEND_PORT="${BACKEND_PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
BACKEND_HOST="${BACKEND_HOST:-0.0.0.0}"
FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
DATA_DIR="${DB_PATH:-$ROOT_DIR/data/modem.db}"
FRONTEND_DIR="$ROOT_DIR/webui"
API_TARGET="http://127.0.0.1:$BACKEND_PORT"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

BACKEND_PID=""
FRONTEND_PID=""
BACKEND_PGID=""
FRONTEND_PGID=""
CLEANED_UP=0

log_info() { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

process_group_id() {
    local pid=$1
    ps -o pgid= -p "$pid" 2>/dev/null | tr -d '[:space:]' || true
}

capture_process_group() {
    local pid=$1
    local pgid=""

    for _ in $(seq 1 10); do
        pgid="$(process_group_id "$pid")"
        if [ -n "$pgid" ]; then
            echo "$pgid"
            return
        fi
        sleep 0.1
    done
}

descendant_pids() {
    local parent=$1
    local children=""
    local child

    if ! command -v pgrep >/dev/null 2>&1; then
        return
    fi

    children="$(pgrep -P "$parent" 2>/dev/null || true)"
    for child in $children; do
        descendant_pids "$child"
        echo "$child"
    done
}

port_listeners() {
    local port=$1

    if ! command -v lsof >/dev/null 2>&1; then
        return
    fi

    lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true
}

service_is_running() {
    local pid="${1:-}"
    local port="${2:-}"
    local pids=""

    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        return 0
    fi

    if [ -n "$port" ]; then
        pids="$(port_listeners "$port")"
        if [ -n "$pids" ]; then
            return 0
        fi
    fi

    return 1
}

send_signal_to_service() {
    local signal=$1
    local pid="${2:-}"
    local pgid="${3:-}"
    local self_pgid=""
    local child

    self_pgid="$(process_group_id "$$")"
    if [ -n "$pgid" ] && [ -n "$self_pgid" ] && [ "$pgid" != "$self_pgid" ]; then
        kill "-$signal" "-$pgid" 2>/dev/null || true
    fi

    if [ -n "$pid" ]; then
        for child in $(descendant_pids "$pid"); do
            kill "-$signal" "$child" 2>/dev/null || true
        done
        kill "-$signal" "$pid" 2>/dev/null || true
    fi
}

wait_for_service_stop() {
    local pid="${1:-}"
    local port="${2:-}"
    local attempts=$3

    for _ in $(seq 1 "$attempts"); do
        if ! service_is_running "$pid" "$port"; then
            return 0
        fi
        sleep 0.25
    done

    return 1
}

terminate_port_listeners() {
    local signal=$1
    local port="${2:-}"
    local name=$3
    local pids=""
    local pid

    if [ -z "$port" ]; then
        return
    fi

    pids="$(port_listeners "$port")"
    if [ -z "$pids" ]; then
        return
    fi

    log_warn "端口 $port 仍被占用（${name}），发送 ${signal} 信号: $(echo "$pids" | tr '\n' ' ')"
    for pid in $pids; do
        kill "-$signal" "$pid" 2>/dev/null || true
    done
}

stop_service() {
    local name=$1
    local pid="${2:-}"
    local pgid="${3:-}"
    local port="${4:-}"
    local initial_port_pids=""

    if [ -n "$port" ]; then
        initial_port_pids="$(port_listeners "$port")"
    fi

    if [ -z "$pid" ] && [ -z "$pgid" ] && [ -z "$initial_port_pids" ]; then
        return
    fi

    send_signal_to_service TERM "$pid" "$pgid"
    if wait_for_service_stop "$pid" "$port" 10; then
        [ -n "$pid" ] && wait "$pid" 2>/dev/null || true
        log_info "${name}已停止"
        return
    fi

    terminate_port_listeners TERM "$port" "$name"
    if wait_for_service_stop "$pid" "$port" 10; then
        [ -n "$pid" ] && wait "$pid" 2>/dev/null || true
        log_info "${name}已停止"
        return
    fi

    log_warn "${name}未在限定时间内退出，强制停止"
    send_signal_to_service KILL "$pid" "$pgid"
    terminate_port_listeners KILL "$port" "$name"
    if wait_for_service_stop "$pid" "$port" 10; then
        [ -n "$pid" ] && wait "$pid" 2>/dev/null || true
        log_info "${name}已停止"
        return
    fi

    log_error "${name}停止失败，请手动检查端口 $port"
}

cleanup() {
    local status=$?
    if [ "$CLEANED_UP" -eq 1 ]; then
        exit "$status"
    fi
    CLEANED_UP=1

    echo ""
    log_info "正在停止服务..."

    stop_service "Vite 前端" "$FRONTEND_PID" "$FRONTEND_PGID" "$FRONTEND_PORT"
    stop_service "Go 后端" "$BACKEND_PID" "$BACKEND_PGID" "$BACKEND_PORT"

    if [ "$status" -eq 0 ]; then
        log_info "所有服务已停止，再见！"
    fi
    exit "$status"
}

check_command() {
    local cmd=$1
    if ! command -v "$cmd" >/dev/null 2>&1; then
        log_error "未找到命令: $cmd，请先安装"
        exit 1
    fi
}

release_port() {
    local port=$1
    local name=$2
    local pids=""
    local pid

    if ! command -v lsof >/dev/null 2>&1; then
        log_warn "未找到 lsof，跳过端口 $port 检查"
        return
    fi

    if ! pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null)"; then
        return
    fi
    if [ -z "$pids" ]; then
        return
    fi

    log_warn "端口 $port 已被占用（${name}），正在结束占用进程: $(echo "$pids" | tr '\n' ' ')"
    for pid in $pids; do
        kill "$pid" 2>/dev/null || true
    done

    for _ in $(seq 1 20); do
        if ! lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
            log_info "端口 $port 已释放"
            return
        fi
        sleep 0.25
    done

    pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
    if [ -n "$pids" ]; then
        log_warn "端口 $port 仍被占用，强制结束进程: $(echo "$pids" | tr '\n' ' ')"
        for pid in $pids; do
            kill -9 "$pid" 2>/dev/null || true
        done
    fi

    for _ in $(seq 1 10); do
        if ! lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
            log_info "端口 $port 已释放"
            return
        fi
        sleep 0.25
    done

    if lsof -iTCP:"$port" -sTCP:LISTEN -nP >/dev/null 2>&1; then
        log_error "端口 $port 仍被占用（${name}），无法自动释放"
        lsof -iTCP:"$port" -sTCP:LISTEN -nP || true
        exit 1
    fi
}

wait_for_url() {
    local url=$1
    local name=$2
    local pid=$3
    local timeout_seconds=$4

    for _ in $(seq 1 "$timeout_seconds"); do
        if ! kill -0 "$pid" 2>/dev/null; then
            log_error "$name 启动失败"
            exit 1
        fi

        if command -v curl >/dev/null 2>&1 && curl -fsS "$url" >/dev/null 2>&1; then
            return
        fi

        sleep 1
    done

    if command -v curl >/dev/null 2>&1; then
        log_error "$name 启动超时: $url"
        exit 1
    fi

    log_warn "未找到 curl，已跳过 $name 健康检查"
}

monitor_services() {
    local backend_status=0
    local frontend_status=0

    while true; do
        if [ -n "$BACKEND_PID" ] && ! kill -0 "$BACKEND_PID" 2>/dev/null; then
            wait "$BACKEND_PID" 2>/dev/null || backend_status=$?
            log_warn "Go 后端已退出，正在停止 Vite 前端"
            return "$backend_status"
        fi

        if [ -n "$FRONTEND_PID" ] && ! kill -0 "$FRONTEND_PID" 2>/dev/null; then
            wait "$FRONTEND_PID" 2>/dev/null || frontend_status=$?
            log_warn "Vite 前端已退出，正在停止 Go 后端"
            return "$frontend_status"
        fi

        sleep 1
    done
}

log_info "========== Web Modem 开发环境 =========="

check_command go
check_command node
check_command corepack

if [ ! -d "$FRONTEND_DIR" ]; then
    log_error "前端目录不存在: $FRONTEND_DIR"
    exit 1
fi

release_port "$BACKEND_PORT" "Go 后端"
release_port "$FRONTEND_PORT" "Vite 前端"

trap cleanup SIGINT SIGTERM EXIT
set -m 2>/dev/null || true

if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
    log_info "首次运行，安装前端依赖..."
    (cd "$FRONTEND_DIR" && corepack pnpm install)
    log_info "前端依赖安装完成"
fi

# webview/dist 由 Go //go:embed 打包，已 gitignore；首次运行需生成一次以便后端编译。
# 开发态前端实际由 Vite 提供，此处仅为满足 embed 编译。
if [ ! -f "$ROOT_DIR/webview/dist/index.html" ]; then
    log_info "生成前端构建产物（供 Go embed）..."
    (cd "$FRONTEND_DIR" && corepack pnpm build)
fi

log_info "启动 Go 后端 (端口: ${BACKEND_PORT})..."
(
    cd "$ROOT_DIR"
    exec env \
        HTTP_PORT="$BACKEND_PORT" \
        HOST="$BACKEND_HOST" \
        DB_PATH="$DATA_DIR" \
        go run .
) &
BACKEND_PID=$!
BACKEND_PGID="$(capture_process_group "$BACKEND_PID")"

wait_for_url "${API_TARGET}/api/modem/list" "Go 后端" "$BACKEND_PID" 30
log_info "Go 后端已启动 ${CYAN}${API_TARGET}${NC}"

log_info "启动 Vite 前端开发服务器 (端口: ${FRONTEND_PORT})..."
(
    cd "$FRONTEND_DIR"
    exec env \
        WEB_MODEM_DEV_API_TARGET="$API_TARGET" \
        corepack pnpm exec vite --host "$FRONTEND_HOST" --port "$FRONTEND_PORT"
) &
FRONTEND_PID=$!
FRONTEND_PGID="$(capture_process_group "$FRONTEND_PID")"

wait_for_url "http://127.0.0.1:${FRONTEND_PORT}" "Vite 前端" "$FRONTEND_PID" 30
log_info "Vite 前端已启动 ${CYAN}http://localhost:${FRONTEND_PORT}${NC}"

echo ""
log_info "=========================================="
log_info "开发环境已就绪！"
log_info ""
log_info "  前端地址:  ${CYAN}http://localhost:${FRONTEND_PORT}${NC}"
log_info "  后端地址:  ${CYAN}${API_TARGET}${NC}"
log_info "  API 检查:  ${CYAN}${API_TARGET}/api/modem/list${NC}"
log_info ""
log_info "  数据库:    ${CYAN}${DATA_DIR}${NC}"
log_info ""
log_info "  按 ${YELLOW}Ctrl+C${NC} 停止所有服务"
log_info "=========================================="
echo ""

monitor_services
