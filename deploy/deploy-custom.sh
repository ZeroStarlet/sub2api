#!/bin/bash
#
# Sub2API Custom Deployment Script (ZeroStarlet Fork)
# 自定义部署脚本 — 从源码编译部署（含遥测隐私等定制功能）
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/ZeroStarlet/sub2api/main/deploy/deploy-custom.sh | bash
#   or locally:
#   sudo bash deploy/deploy-custom.sh
#
# Features:
#   - 从你的 fork 克隆源码
#   - 编译前端 (pnpm) + 后端 (go, embed 前端)
#   - 自动配置 systemd 服务
#   - 支持更新/重新部署（增量编译）
#

set -e

# ============================================================
# Configuration — 按需修改
# ============================================================

GITHUB_REPO="ZeroStarlet/sub2api"          # 你的 fork
DEPLOY_BRANCH="feature/telemetry-privacy"   # 部署分支（遥测隐私功能）
# DEPLOY_BRANCH="main"                      # 或跟随上游 main

APP_DIR="/opt/sub2api"                      # 应用目录
CONFIG_DIR="/etc/sub2api"                   # 配置目录
SERVICE_NAME="sub2api"                      # systemd 服务名
SERVICE_USER="sub2api"                      # 运行用户

# Go 编译参数
GO_VERSION_MIN="1.23"                       # 最低 Go 版本
CGO_ENABLED="${CGO_ENABLED:-0}"             # 通常关闭 CGO

# Node.js 参数
NODE_VERSION_MIN="20"                       # 最低 Node 版本

# ============================================================
# 颜色输出
# ============================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $1"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# ============================================================
# 权限检查
# ============================================================
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        error "请使用 root 权限运行: sudo bash $0"
    fi
}

# ============================================================
# 系统依赖检查
# ============================================================
check_deps() {
    info "检查系统依赖..."

    local missing=()

    # Go
    if command -v go &>/dev/null; then
        local go_ver=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' | head -1)
        if [ "$(printf '%s\n' "$GO_VERSION_MIN" "$go_ver" | sort -V | head -1)" != "$GO_VERSION_MIN" ]; then
            warn "Go 版本: $go_ver (需要 >= $GO_VERSION_MIN)"
            missing+=("go(>=${GO_VERSION_MIN})")
        else
            ok "Go: $go_ver"
        fi
    else
        missing+=("go")
    fi

    # Node.js + pnpm
    if command -v node &>/dev/null; then
        local node_ver=$(node -v | grep -oP '\d+' | head -1)
        if [ "$node_ver" -lt "$NODE_VERSION_MIN" ]; then
            warn "Node.js 版本: $(node -v) (需要 >= $NODE_VERSION_MIN)"
            missing+=("nodejs(>=${NODE_VERSION_MIN})")
        else
            ok "Node.js: $(node -v)"
        fi
    else
        missing+=("nodejs")
    fi

    if command -v pnpm &>/dev/null; then
        ok "pnpm: $(pnpm -v)"
    else
        missing+=("pnpm")
    fi

    # git
    if command -v git &>/dev/null; then
        ok "git: $(git version | awk '{print $3}')"
    else
        missing+=("git")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        echo ""
        error "缺少依赖: ${missing[*]}

请安装后重试:
  # Ubuntu/Debian
  sudo apt update && sudo apt install -y golang-go nodejs npm git
  sudo npm install -g pnpm

  # 或使用官方安装方式安装 Go/Node.js 最新版"
    fi
}

# ============================================================
# 克隆 / 更新源码
# ============================================================
clone_or_update() {
    local repo_url="https://github.com/${GITHUB_REPO}.git"

    if [ -d "$APP_DIR/.git" ]; then
        info "源码目录已存在，更新到最新..."
        cd "$APP_DIR"
        git fetch origin
        git checkout "$DEPLOY_BRANCH"
        git pull origin "$DEPLOY_BRANCH" 2>/dev/null || git reset --hard "origin/$DEPLOY_BRANCH"
        ok "代码已更新: $(git log --oneline -1)"
    else
        info "克隆仓库: $repo_url"
        mkdir -p "$(dirname "$APP_DIR")"
        git clone -b "$DEPLOY_BRANCH" "$repo_url" "$APP_DIR"
        cd "$APP_DIR"
        ok "代码已克隆: $(git log --oneline -1)"
    fi

    # 设置 upstream 以便后续合并上游更新
    if ! git remote | grep -q "^upstream$"; then
        git remote add upstream https://github.com/Wei-Shaw/sub2api.git
        info "已添加 upstream 远程仓库"
    fi
}

# ============================================================
# 编译前端
# ============================================================
build_frontend() {
    info "编译前端..."
    cd "$APP_DIR/frontend"

    if [ ! -d "node_modules" ]; then
        info "安装前端依赖..."
        pnpm install --frozen-lockfile
    else
        info "前端依赖已存在，跳过安装"
    fi

    pnpm run build

    if [ -d "$APP_DIR/backend/internal/web/dist" ]; then
        ok "前端构建完成 → backend/internal/web/dist/"
    else
        error "前端构建失败，输出目录不存在"
    fi
}

# ============================================================
# 编译后端
# ============================================================
build_backend() {
    info "编译后端..."
    cd "$APP_DIR/backend"

    # 确保 Go 依赖
    if [ ! -f "go.sum" ]; then
        error "go.sum 不存在，请检查仓库完整性"
    fi

    info "下载 Go 依赖..."
    go mod download

    # 编译（嵌入前端静态文件）
    local version
    version=$(tr -d '\r\n' < ./cmd/server/VERSION 2>/dev/null || echo "dev")
    local ldflags="-s -w -X main.Version=${version} -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"

    info "开始编译 (CGO_ENABLED=$CGO_ENABLED, version=$version)..."
    CGO_ENABLED="$CGO_ENABLED" \
        go build -tags embed -ldflags="$ldflags" -trimpath -o "$APP_DIR/sub2api" ./cmd/server

    chmod +x "$APP_DIR/sub2api"
    ok "后端编译完成: $APP_DIR/sub2api ($version)"
}

# ============================================================
# 创建系统用户
# ============================================================
create_user() {
    if id "$SERVICE_USER" &>/dev/null; then
        info "用户已存在: $SERVICE_USER"
        # 确保 shell 可用（之前版本可能设为 /bin/false）
        local current_shell
        current_shell=$(getent passwd "$SERVICE_USER" 2>/dev/null | cut -d: -f7)
        if [ "$current_shell" = "/bin/false" ] || [ "$current_shell" = "/sbin/nologin" ]; then
            usermod -s /bin/sh "$SERVICE_USER" 2>/dev/null || true
        fi
    else
        info "创建系统用户: $SERVICE_USER"
        useradd -r -s /bin/sh -d "$APP_DIR" "$SERVICE_USER"
    fi
}

# ============================================================
# 配置目录权限
# ============================================================
setup_dirs() {
    info "设置目录权限..."
    mkdir -p "$CONFIG_DIR"
    chown -R "$SERVICE_USER:$SERVICE_USER" "$APP_DIR"
    chown -R "$SERVICE_USER:$SERVICE_USER" "$CONFIG_DIR"
    ok "目录权限设置完成"
}

# ============================================================
# 配置文件引导
# ============================================================
setup_config() {
    if [ -f "$CONFIG_DIR/config.yaml" ]; then
        info "配置文件已存在: $CONFIG_DIR/config.yaml (跳过)"
        return
    fi

    warn "配置文件不存在。"
    info "从模板创建..."

    if [ -f "$APP_DIR/deploy/config.example.yaml" ]; then
        cp "$APP_DIR/deploy/config.example.yaml" "$CONFIG_DIR/config.yaml"
        ok "已创建: $CONFIG_DIR/config.yaml"
        warn "请编辑配置文件中的数据库和 Redis 连接信息:"
        echo ""
        echo "  sudo nano $CONFIG_DIR/config.yaml"
        echo ""
    else
        warn "模板文件不存在，请手动创建配置"
    fi
}

# ============================================================
# systemd 服务
# ============================================================
install_service() {
    info "安装 systemd 服务..."

    cat > "/etc/systemd/system/${SERVICE_NAME}.service" << EOF
[Unit]
Description=Sub2API - AI API Gateway Platform (Custom Fork)
Documentation=https://github.com/${GITHUB_REPO}
After=network.target postgresql.service redis.service
Wants=postgresql.service redis.service

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
WorkingDirectory=${APP_DIR}
ExecStart=${APP_DIR}/sub2api
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

# 安全加固
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=${APP_DIR}
ReadWritePaths=${CONFIG_DIR}

# 环境变量
Environment=GIN_MODE=release

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    ok "systemd 服务已安装: ${SERVICE_NAME}"
}

# ============================================================
# 启动服务
# ============================================================
start_service() {
    info "启动服务..."

    # 先停掉旧实例
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        info "停止旧服务..."
        systemctl stop "$SERVICE_NAME"
    fi

    if systemctl start "$SERVICE_NAME"; then
        ok "服务已启动"
    else
        error "服务启动失败，查看日志: sudo journalctl -u ${SERVICE_NAME} -n 50"
    fi

    systemctl enable "$SERVICE_NAME" 2>/dev/null || true
    ok "已设置开机自启"
}

# ============================================================
# 状态检查
# ============================================================
check_status() {
    sleep 2
    echo ""
    echo "=============================================="
    info "部署完成 — 服务状态:"
    echo "=============================================="
    echo ""

    systemctl status "$SERVICE_NAME" --no-pager -l 2>/dev/null || true

    echo ""
    echo "=============================================="
    echo "  常用命令"
    echo "=============================================="
    echo ""
    echo "  查看状态:   sudo systemctl status ${SERVICE_NAME}"
    echo "  查看日志:   sudo journalctl -u ${SERVICE_NAME} -f"
    echo "  重启服务:   sudo systemctl restart ${SERVICE_NAME}"
    echo "  停止服务:   sudo systemctl stop ${SERVICE_NAME}"
    echo ""
    echo "  更新重新部署:"
    echo "    sudo bash ${APP_DIR}/deploy/deploy-custom.sh"
    echo ""
    echo "  合并上游更新:"
    echo "    cd ${APP_DIR}"
    echo "    git fetch upstream"
    echo "    git merge upstream/main"
    echo "    sudo bash ${APP_DIR}/deploy/deploy-custom.sh"
    echo ""
}

# ============================================================
# 主流程
# ============================================================
main() {
    echo ""
    echo -e "${CYAN}=============================================="
    echo "  Sub2API 自定义部署脚本"
    echo "  仓库: ${GITHUB_REPO}"
    echo "  分支: ${DEPLOY_BRANCH}"
    echo "==============================================${NC}"
    echo ""

    check_root
    check_deps
    clone_or_update
    create_user
    setup_dirs
    build_frontend
    build_backend
    setup_config
    install_service
    start_service
    check_status
}

main "$@"
