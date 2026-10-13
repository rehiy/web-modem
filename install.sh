#!/bin/sh

set -eu
set -f

APP_NAME="${APP_NAME:-web-modem}"
GITHUB_API_BASE="${GITHUB_API_BASE:-https://api.github.com}"
GITHUB_REPO="${GITHUB_REPO:-rehiy/web-modem}"

INSTALL_DIR="${INSTALL_DIR:-}"
ENV_FILE="${ENV_FILE:-}"
DB_PATH_VALUE="${DB_PATH:-}"
HTTP_PORT_VALUE="${HTTP_PORT:-8080}"
INSTALL_SERVICE="${INSTALL_SERVICE:-auto}"
START_SERVICE="${START_SERVICE:-1}"
SERVICE_RUNNER="${SERVICE_RUNNER:-}"
LAUNCHD_PLIST="${LAUNCHD_PLIST:-}"
LOG_DIR="${LOG_DIR:-}"

AUTH_CREATED=0
AUTH_ENABLED=""
AUTH_MODE_VALUE=""
AUTH_USER_VALUE=""
AUTH_PASSWORD_VALUE=""

fail() {
    echo "error: $*" >&2
    exit 1
}

info() {
    echo "==> $*"
}

need_cmd() {
    command -v "$1" >/dev/null 2>&1 || fail "缺少命令: $1"
}

run_root() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
        return
    fi

    command -v sudo >/dev/null 2>&1 || fail "需要 root 权限，请使用 root 执行或安装 sudo"
    sudo "$@"
}

run_install() {
    install_target="$1"
    shift
    install_parent="$install_target"
    while [ ! -d "$install_parent" ]; do
        install_parent="$(dirname "$install_parent")"
    done

    # 用户目录中的安装保持当前用户所有权，系统路径才需要提权。
    if [ -w "$install_parent" ] && { [ ! -e "$install_target" ] || [ -w "$install_target" ]; }; then
        "$@"
    else
        run_root "$@"
    fi
}

install_dir() {
    # 不修改已有目录的权限，例如用户的私有配置目录。
    [ -d "$1" ] && return 0
    run_install "$1" install -d -m 0755 "$1"
}

detect_platform() {
    case "$(uname -s)" in
        Linux) os="linux" ;;
        Darwin) os="darwin" ;;
        *) fail "当前一键安装脚本只支持 Linux/macOS" ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        armv7l|armv6l)
            [ "$os" = "linux" ] || fail "macOS 不支持的 CPU 架构: $(uname -m)"
            arch="arm"
            ;;
        ppc64le)
            [ "$os" = "linux" ] || fail "macOS 不支持的 CPU 架构: $(uname -m)"
            arch="ppc64le"
            ;;
        s390x)
            [ "$os" = "linux" ] || fail "macOS 不支持的 CPU 架构: $(uname -m)"
            arch="s390x"
            ;;
        *) fail "不支持的 CPU 架构: $(uname -m)" ;;
    esac
}

set_platform_defaults() {
    if [ "$os" = "darwin" ]; then
        INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
        ENV_FILE="${ENV_FILE:-/usr/local/etc/web-modem.env}"
        DB_PATH_VALUE="${DB_PATH_VALUE:-/usr/local/var/lib/web-modem/modem.db}"
        SERVICE_RUNNER="${SERVICE_RUNNER:-/usr/local/libexec/web-modem-run}"
        LAUNCHD_PLIST="${LAUNCHD_PLIST:-/Library/LaunchDaemons/$APP_NAME.plist}"
        LOG_DIR="${LOG_DIR:-/usr/local/var/log}"
        return
    fi

    INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
    ENV_FILE="${ENV_FILE:-/etc/default/web-modem}"
    DB_PATH_VALUE="${DB_PATH_VALUE:-/var/lib/web-modem/modem.db}"
}

random_password() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 12
        return
    fi

    LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24
    echo
}

parse_release() {
    release_file="$1"
    asset_name="$2"

    if command -v python3 >/dev/null 2>&1; then
        python3 - "$release_file" "$asset_name" <<'PY'
import json
import sys

release_file, asset_name = sys.argv[1], sys.argv[2]
with open(release_file, "r", encoding="utf-8") as f:
    data = json.load(f)

asset = None
for item in data.get("assets") or []:
    if item.get("name") == asset_name:
        asset = item
        break

if asset is None:
    print(f"未找到当前平台安装包: {asset_name}", file=sys.stderr)
    sys.exit(1)

url = asset.get("browser_download_url") or ""
if not url:
    print(f"安装包缺少下载地址: {asset_name}", file=sys.stderr)
    sys.exit(1)

print(data.get("tag_name") or data.get("name") or "latest")
print(url)
digest = asset.get("digest") or ""
print("sha256" if digest.startswith("sha256:") else "")
print(digest[len("sha256:"): ] if digest.startswith("sha256:") else "")
checksums = next((item.get("browser_download_url", "") for item in data.get("assets", []) if item.get("name") == "checksums.txt"), "")
print(checksums)
PY
        return
    fi

    json_line="$(tr -d '\n\r' < "$release_file")"
    version="$(printf '%s' "$json_line" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
    asset_line="$(printf '%s' "$json_line" | sed 's/}[[:space:]]*,[[:space:]]*{/},\
{/g' | grep "\"name\"[[:space:]]*:[[:space:]]*\"$asset_name\"" | head -n 1 || true)"

    [ -n "$asset_line" ] || fail "未找到当前平台安装包: $asset_name"

    asset_url="$(printf '%s' "$asset_line" | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
    digest="$(printf '%s' "$asset_line" | sed -n 's/.*"digest"[[:space:]]*:[[:space:]]*"\([^" ]*\)".*/\1/p')"
    hash_algo=""
    hash_value=""
    case "$digest" in
        sha256:*) hash_algo="sha256"; hash_value="${digest#sha256:}" ;;
    esac
    checksum_line="$(printf '%s' "$json_line" | sed 's/}[[:space:]]*,[[:space:]]*{/},\
{/g' | grep '\"name\"[[:space:]]*:[[:space:]]*\"checksums.txt\"' | head -n 1 || true)"
    checksum_url="$(printf '%s' "$checksum_line" | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^" ]*\)".*/\1/p')"

    [ -n "$asset_url" ] || fail "安装包缺少下载地址: $asset_name"
    printf '%s\n%s\n%s\n%s\n%s\n' "${version:-latest}" "$asset_url" "$hash_algo" "$hash_value" "$checksum_url"
}

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
        return
    fi

    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
        return
    fi

    echo ""
}

# AUTH_MODE: auto（默认，按是否提供 BASIC_AUTH_PASSWORD 推断）/ on（强制鉴权）/ off（无鉴权）
# 归一化取值并校验，同时把结果写入 AUTH_ENABLED/AUTH_MODE_VALUE 供安装总结使用
resolve_auth_mode() {
    case "$(printf '%s' "${AUTH_MODE:-auto}" | tr '[:upper:]' '[:lower:]')" in
        auto|"")
            if [ -n "${BASIC_AUTH_PASSWORD:-}" ]; then
                AUTH_MODE_VALUE="on"
            else
                AUTH_MODE_VALUE="off"
            fi
            ;;
        on|1|true|yes) AUTH_MODE_VALUE="on" ;;
        off|0|false|no) AUTH_MODE_VALUE="off" ;;
        *) fail "AUTH_MODE 只能是 auto/on/off" ;;
    esac

    if [ "$AUTH_MODE_VALUE" = "on" ]; then
        AUTH_ENABLED=1
    else
        AUTH_ENABLED=0
    fi
}

write_env_file() {
    resolve_auth_mode

    if [ -f "$ENV_FILE" ]; then
        info "配置文件已存在，跳过覆盖: $ENV_FILE"
        return
    fi

    env_tmp="$tmp_dir/web-modem.env"
    if [ "$AUTH_ENABLED" = "1" ]; then
        AUTH_USER_VALUE="${BASIC_AUTH_USER:-admin}"
        AUTH_PASSWORD_VALUE="${BASIC_AUTH_PASSWORD:-$(random_password)}"
        AUTH_CREATED=1

        cat > "$env_tmp" <<EOF
DB_PATH=$DB_PATH_VALUE
HTTP_PORT=$HTTP_PORT_VALUE
BASIC_AUTH_USER=$AUTH_USER_VALUE
BASIC_AUTH_PASSWORD=$AUTH_PASSWORD_VALUE
# MODEM_PORT=/dev/ttyUSB0
EOF
    else
        # 无鉴权模式：不写入 BASIC_AUTH_*，服务端未配置时直接放行
        cat > "$env_tmp" <<EOF
DB_PATH=$DB_PATH_VALUE
HTTP_PORT=$HTTP_PORT_VALUE
# BASIC_AUTH_USER=
# BASIC_AUTH_PASSWORD=
# MODEM_PORT=/dev/ttyUSB0
EOF
    fi

    install_dir "$(dirname "$ENV_FILE")"
    run_install "$ENV_FILE" install -m 0600 "$env_tmp" "$ENV_FILE"
}

install_systemd_service() {
    service_tmp="$tmp_dir/web-modem.service"
    cat > "$service_tmp" <<EOF
[Unit]
Description=Web Modem
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-$ENV_FILE
WorkingDirectory=$(dirname "$DB_PATH_VALUE")
ExecStart=$INSTALL_DIR/$APP_NAME
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

    run_root install -m 0644 "$service_tmp" "/etc/systemd/system/$APP_NAME.service"
    run_root systemctl daemon-reload
    run_root systemctl enable "$APP_NAME.service" >/dev/null

    if [ "$START_SERVICE" != "0" ]; then
        run_root systemctl restart "$APP_NAME.service"
    fi
}

launchd_bootout() {
    if [ "$(id -u)" -eq 0 ]; then
        launchctl bootout system "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
        return
    fi

    sudo launchctl bootout system "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
}

install_launchd_service() {
    runner_tmp="$tmp_dir/web-modem-run"
    cat > "$runner_tmp" <<EOF
#!/bin/sh
set -a
[ -f "$ENV_FILE" ] && . "$ENV_FILE"
set +a
work_dir=\$(dirname "\${DB_PATH:-$DB_PATH_VALUE}")
mkdir -p "\$work_dir"
cd "\$work_dir" || exit 1
exec "$INSTALL_DIR/$APP_NAME"
EOF

    plist_tmp="$tmp_dir/web-modem.plist"
    cat > "$plist_tmp" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$APP_NAME</string>
    <key>ProgramArguments</key>
    <array>
        <string>$SERVICE_RUNNER</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$LOG_DIR/$APP_NAME.log</string>
    <key>StandardErrorPath</key>
    <string>$LOG_DIR/$APP_NAME.err.log</string>
</dict>
</plist>
EOF

    run_root install -d -m 0755 "$(dirname "$SERVICE_RUNNER")"
    run_root install -m 0755 "$runner_tmp" "$SERVICE_RUNNER"
    run_root install -d -m 0755 "$LOG_DIR"
    run_root install -d -m 0755 "$(dirname "$LAUNCHD_PLIST")"
    run_root install -m 0644 "$plist_tmp" "$LAUNCHD_PLIST"

    if [ "$START_SERVICE" != "0" ]; then
        launchd_bootout
        run_root launchctl bootstrap system "$LAUNCHD_PLIST"
        run_root launchctl kickstart -k "system/$APP_NAME"
    fi
}

install_service() {
    if [ "$os" = "darwin" ]; then
        install_launchd_service
        return
    fi

    install_systemd_service
}

should_install_service() {
    case "$INSTALL_SERVICE" in
        1|true|yes|on) return 0 ;;
        0|false|no|off) return 1 ;;
        auto)
            if [ "$os" = "darwin" ]; then
                command -v launchctl >/dev/null 2>&1
                return
            fi

            command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]
            return
            ;;
        *) fail "INSTALL_SERVICE 只能是 auto/1/0" ;;
    esac
}

need_cmd curl
need_cmd gzip
need_cmd install
detect_platform
set_platform_defaults

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

asset_name="$APP_NAME-$os-$arch.gz"
release_file="$tmp_dir/release.json"
archive_file="$tmp_dir/$asset_name"
binary_file="$tmp_dir/$APP_NAME"

info "查询最新版本"
curl -fsSL --retry 3 --retry-delay 2 \
    -H "Accept: application/vnd.github+json" \
    "$GITHUB_API_BASE/repos/$GITHUB_REPO/releases/latest" \
    -o "$release_file"

release_info="$(parse_release "$release_file" "$asset_name")"
release_version="$(printf '%s\n' "$release_info" | sed -n '1p')"
asset_url="$(printf '%s\n' "$release_info" | sed -n '2p')"
hash_algo="$(printf '%s\n' "$release_info" | sed -n '3p')"
hash_value="$(printf '%s\n' "$release_info" | sed -n '4p')"
checksum_url="$(printf '%s\n' "$release_info" | sed -n '5p')"

if [ -z "$hash_value" ] && [ -n "$checksum_url" ]; then
    curl -fsSL --retry 3 --retry-delay 2 "$checksum_url" -o "$tmp_dir/checksums.txt"
    hash_value="$(awk -v name="$asset_name" '{file=$2; sub(/^\*/, "", file); if (file == name) print $1}' "$tmp_dir/checksums.txt")"
    [ -n "$hash_value" ] || fail "校验清单缺少安装包: $asset_name"
    hash_algo="sha256"
fi

info "下载 $asset_name ($release_version)"
curl -fL --retry 3 --retry-delay 2 \
    -H "Accept: application/octet-stream" \
    "$asset_url" \
    -o "$archive_file"

if [ "$(printf '%s' "$hash_algo" | tr '[:upper:]' '[:lower:]')" = "sha256" ] && [ -n "$hash_value" ]; then
    actual_hash="$(sha256_file "$archive_file")"
    if [ -n "$actual_hash" ]; then
        [ "$actual_hash" = "$hash_value" ] || fail "SHA256 校验失败"
        info "SHA256 校验通过"
    else
        info "未找到 sha256sum/shasum，跳过 SHA256 校验"
    fi
fi

gzip -dc "$archive_file" > "$binary_file"
chmod 0755 "$binary_file"

info "安装到 $INSTALL_DIR/$APP_NAME"
install_dir "$INSTALL_DIR"
run_install "$INSTALL_DIR/$APP_NAME" install -m 0755 "$binary_file" "$INSTALL_DIR/$APP_NAME"
install_dir "$(dirname "$DB_PATH_VALUE")"
write_env_file

if should_install_service; then
    if [ "$os" = "darwin" ]; then
        info "安装 launchd 服务"
    else
        info "安装 systemd 服务"
    fi
    install_service
    if [ "$START_SERVICE" != "0" ]; then
        if [ "$os" = "darwin" ]; then
            info "服务已启动: system/$APP_NAME"
        else
            info "服务已启动: $APP_NAME.service"
        fi
    else
        if [ "$os" = "darwin" ]; then
            info "服务已安装但未启动: sudo launchctl bootstrap system $LAUNCHD_PLIST"
        else
            info "服务已安装但未启动: systemctl start $APP_NAME.service"
        fi
    fi
else
    info "跳过服务安装"
fi

echo
echo "安装完成: $release_version"
echo "访问地址: http://localhost:$HTTP_PORT_VALUE"
echo "配置文件: $ENV_FILE"
if [ "$AUTH_ENABLED" = "1" ]; then
    if [ "$AUTH_CREATED" = "1" ]; then
        echo "认证账号: $AUTH_USER_VALUE"
        echo "认证密码: $AUTH_PASSWORD_VALUE"
    else
        echo "认证模式: Basic Auth（沿用已有配置）"
    fi
    echo "提示: 不需要鉴权时可用 AUTH_MODE=off 重装（配置已存在时需先备份并删除）"
else
    echo "认证模式: 无鉴权（未启用 Basic Auth）"
    echo "提示: 需要鉴权时可用 AUTH_MODE=on 重装（配置已存在时需先备份并删除）"
fi
