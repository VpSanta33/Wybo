#!/usr/bin/env bash
# Native Wybo deployment. See docs/原生部署.md. No Docker is used.
set +x
set -Eeuo pipefail
umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="$SCRIPT_DIR"
HELPER="$SCRIPT_DIR/deploy/render.py"
STATE=/etc/wybo/deployment.json
APP_ROOT=/opt/wybo
DOMAIN=""
EMAIL=""
ADMIN_USER=""
OUTPUT_DIR=""
DRY_RUN=0
ASSUME_YES=0
ACTION=install
STEP=初始化

log() { printf '\n[Wybo] %s\n' "$*"; }
fetch() { curl --fail --silent --show-error --location --retry 3 --connect-timeout 15 --max-time 900 "$@"; }
die() { printf '\n[Wybo] 错误：%s\n' "$*" >&2; exit 1; }
on_error() {
  local code=$?
  printf '\n[Wybo] %s失败（退出码 %s）。不会重置数据库、密码或 2FA。\n' "$STEP" "$code" >&2
  printf '检查：journalctl -u wybo -n 80；nginx -t。修复后可重新运行本脚本。\n' >&2
  exit "$code"
}
trap on_error ERR

usage() {
  cat <<'USAGE'
用法：sudo bash deploy.sh [install|update|status|backup] [选项]

  --domain DOMAIN      站点域名，例如 shop.example.com（不含 https://）
  --email EMAIL        Let's Encrypt 证书联系邮箱
  --admin-user NAME    首次管理员用户名，默认 admin
  --source-dir PATH    项目源码根目录，默认脚本所在目录
  --yes               不询问部署确认，并同意 Let's Encrypt 订阅协议
  --dry-run           仅在输出目录生成配置；不安装、不联网、不启动服务
  --output-dir PATH   dry-run 输出目录，默认创建临时目录
  -h, --help          显示帮助

支持 Ubuntu 22.04 / 24.04、Debian 12 / 13，systemd，amd64 / arm64。
原生安装 PostgreSQL、Nginx、Certbot、独立 Node 22 / Go 工具链。
重复部署保留数据库和随机凭据；2FA 由管理员在后台自行开启。
USAGE
}

if [[ ${1:-} =~ ^(install|update|status|backup)$ ]]; then ACTION=$1; shift; fi
while (($#)); do
  case "$1" in
    --domain|--email|--admin-user|--source-dir|--output-dir)
      (($# >= 2)) || die "$1 缺少参数"
      case "$1" in
        --domain) DOMAIN=$2 ;; --email) EMAIL=$2 ;; --admin-user) ADMIN_USER=$2 ;;
        --source-dir) SOURCE_DIR=$2 ;; --output-dir) OUTPUT_DIR=$2 ;;
      esac
      shift 2 ;;
    --yes) ASSUME_YES=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数：$1（使用 --help 查看帮助）" ;;
  esac
done

if ! command -v python3 >/dev/null; then
  [[ $EUID == 0 && $DRY_RUN == 0 && -f /etc/os-release ]] || die '此模式需要 Python 3'
  # shellcheck source=/dev/null
  . /etc/os-release
  case "${ID:-}:${VERSION_ID:-}" in
    ubuntu:22.04|ubuntu:24.04|debian:12|debian:13) ;;
    *) die '支持 Ubuntu 22.04 / 24.04、Debian 12 / 13' ;;
  esac
  apt-get -o DPkg::Lock::Timeout=120 update
  DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=120 install -y python3
fi
[[ -f "$HELPER" ]] || die '请上传完整项目，缺少 deploy/render.py'
get_state() { python3 "$HELPER" get --state "$STATE" --key "$1"; }

if [[ "$ACTION" == status || "$ACTION" == backup ]]; then
  ((DRY_RUN == 0)) || die 'status / backup 不接受 --dry-run'
  [[ $EUID == 0 ]] || die '请使用 sudo 或 root 执行'
  [[ -f "$STATE" ]] || die '尚未部署此站点'
  if [[ "$ACTION" == backup ]]; then exec /usr/local/sbin/wybo-backup; fi
  systemctl --no-pager --full status wybo nginx postgresql certbot.timer wybo-backup.timer
  exit 0
fi

if [[ -f "$STATE" && $DRY_RUN == 0 ]]; then
  old_domain=$(get_state domain)
  old_email=$(get_state email)
  old_admin=$(get_state admin_user)
  [[ -z "$DOMAIN" || "$DOMAIN" == "$old_domain" ]] || die '域名与已有部署不同；域名迁移请按文档操作，不会自动覆盖现有站点'
  [[ -z "$ADMIN_USER" || "$ADMIN_USER" == "$old_admin" ]] || die '已有管理员请在后台管理；部署脚本不重设账号'
  DOMAIN=$old_domain
  [[ -z "$EMAIL" || "$EMAIL" == "$old_email" ]] || die '证书邮箱与已有部署不同；请先按文档修改证书账户邮箱'
  EMAIL=$old_email
  ADMIN_USER=$old_admin
elif [[ "$ACTION" == update && $DRY_RUN == 0 ]]; then
  die 'update 需要已有部署；首次请使用 install'
fi

if [[ -z "$DOMAIN" ]]; then
  [[ -t 0 ]] || die '非交互运行必须提供 --domain'
  read -r -p '站点域名（例如 shop.example.com）：' DOMAIN
fi
if [[ -z "$EMAIL" ]]; then
  [[ -t 0 ]] || die '非交互运行必须提供 --email'
  read -r -p '证书联系邮箱：' EMAIL
fi
ADMIN_USER=${ADMIN_USER:-admin}
python3 "$HELPER" validate --domain "$DOMAIN" --email "$EMAIL" --admin-user "$ADMIN_USER"
[[ -f "$SOURCE_DIR/backend/go.mod" && -f "$SOURCE_DIR/frontend/package-lock.json" ]] || die '--source-dir 必须是包含 backend 和 frontend 的完整项目'

if ((DRY_RUN)); then
  OUTPUT_DIR=${OUTPUT_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/wybo-preview.XXXXXXXX")}
  mkdir -p "$OUTPUT_DIR"
  python3 "$HELPER" init --state "$OUTPUT_DIR/deployment.json" --domain "$DOMAIN" --email "$EMAIL" --admin-user "$ADMIN_USER"
  python3 "$HELPER" render --state "$OUTPUT_DIR/deployment.json" --output "$OUTPUT_DIR"
  install -m 700 "$SCRIPT_DIR/deploy/backup.sh" "$OUTPUT_DIR/wybo-backup"
  log "配置预览已生成：${OUTPUT_DIR}（未改动系统，凭据仅供预览）"
  exit 0
fi

[[ $EUID == 0 ]] || die '请使用 sudo bash deploy.sh ...'
[[ -f /etc/os-release ]] || die '仅支持指定版本的 Ubuntu / Debian'
# This is an operating-system-owned file, not a project/user-provided env file.
# shellcheck source=/dev/null
. /etc/os-release
case "${ID:-}:${VERSION_ID:-}" in
  ubuntu:22.04|ubuntu:24.04|debian:12|debian:13) ;;
  *) die '支持 Ubuntu 22.04 / 24.04、Debian 12 / 13' ;;
esac
[[ -d /run/systemd/system ]] || die '需要以 systemd 启动的 Linux 服务器'
case "$(uname -m)" in x86_64) ARCH=amd64; NODE_ARCH=x64 ;; aarch64|arm64) ARCH=arm64; NODE_ARCH=arm64 ;; *) die '仅支持 amd64 / arm64' ;; esac
exec 8>/run/lock/wybo-deploy.lock
flock -n 8 || die '另一个部署任务正在运行，请等待它结束'

if [[ ! -f "$STATE" ]]; then
  [[ ! -e /etc/wybo && ! -e "$APP_ROOT/current" && ! -e /etc/nginx/sites-available/wybo.conf && ! -e /etc/systemd/system/wybo.service ]] || die '发现未经本脚本登记的已有安装，请先核对，避免覆盖'
  python3 "$HELPER" check-port --port 8080
fi
python3 "$HELPER" resolve --domain "$DOMAIN"
log "将原生部署 https://${DOMAIN}，数据库不开放公网；请确保 DNS 指向此服务器且安全组允许 TCP 80 / 443。"
if ((ASSUME_YES == 0)); then
  [[ -t 0 ]] || die '非交互部署需要 --yes'
  printf '申请 SSL 将同意 https://letsencrypt.org/repository/ 的订阅协议。\n'
  read -r -p '开始安装并部署？[y/N] ' answer
  [[ "$answer" == y || "$answer" == Y ]] || exit 0
fi

STEP=安装系统依赖
export DEBIAN_FRONTEND=noninteractive
apt-get -o DPkg::Lock::Timeout=120 update
apt-get -o DPkg::Lock::Timeout=120 install -y --no-install-recommends \
  ca-certificates curl xz-utils openssl python3 rsync nginx postgresql postgresql-contrib certbot util-linux tzdata
systemctl enable --now postgresql nginx
postgres_listen=$(runuser -u postgres -- psql -XwAt --host=/var/run/postgresql --port=5432 --username=postgres --dbname postgres -c 'SHOW listen_addresses')
python3 "$HELPER" check-postgres --listen "$postgres_listen"

if [[ ! -f "$STATE" ]]; then
  existing_database=$(runuser -u postgres -- psql -XwAt --host=/var/run/postgresql --port=5432 --username=postgres --dbname postgres -c "SELECT 1 FROM pg_roles WHERE rolname='wybo' UNION ALL SELECT 1 FROM pg_database WHERE datname='wybo'")
  if [[ -n "$existing_database" ]]; then
    die '发现已有 wybo 数据库或角色；不会覆盖。请使用全新服务器或先按文档人工迁移'
  fi
  install -d -m 750 /etc/wybo
  python3 "$HELPER" init --state "$STATE" --domain "$DOMAIN" --email "$EMAIL" --admin-user "$ADMIN_USER"
fi

STEP=准备系统账户和目录
if ! getent passwd wybo >/dev/null; then useradd --system --user-group --home-dir /var/lib/wybo --shell /usr/sbin/nologin wybo; fi
[[ $(id -u wybo) != 0 ]] || die 'wybo 不能是 root 账户'
install -d -m 750 -o root -g wybo /etc/wybo
install -d -m 755 "$APP_ROOT" "$APP_ROOT/releases" "$APP_ROOT/toolchains" "$APP_ROOT/builds"
install -d -m 750 -o wybo -g wybo /var/lib/wybo /var/lib/wybo/uploads /var/cache/wybo
install -d -m 700 /var/backups/wybo
install -d -m 755 /var/www/letsencrypt/.well-known/acme-challenge

STEP=安装独立构建工具链
TOOL_TMP=$(mktemp -d "$APP_ROOT/toolchains/.download.XXXXXXXX")
fetch 'https://go.dev/dl/?mode=json&include=all' -o "$TOOL_TMP/go.json"
GO_VERSION=$(python3 "$HELPER" version --kind go --metadata "$TOOL_TMP/go.json" --go-mod "$SOURCE_DIR/backend/go.mod")
GO_ARCHIVE="${GO_VERSION}.linux-${ARCH}.tar.gz"
GO_SHA=$(python3 "$HELPER" checksum --metadata "$TOOL_TMP/go.json" --filename "$GO_ARCHIVE")
if [[ ! -x "$APP_ROOT/toolchains/$GO_VERSION/bin/go" ]]; then
  fetch "https://go.dev/dl/$GO_ARCHIVE" -o "$TOOL_TMP/go.tar.gz"
  python3 "$HELPER" verify --file "$TOOL_TMP/go.tar.gz" --sha256 "$GO_SHA"
  tar -xzf "$TOOL_TMP/go.tar.gz" -C "$TOOL_TMP"
  mv "$TOOL_TMP/go" "$APP_ROOT/toolchains/$GO_VERSION"
fi
fetch https://nodejs.org/dist/index.json -o "$TOOL_TMP/node.json"
NODE_VERSION=$(python3 "$HELPER" version --kind node --metadata "$TOOL_TMP/node.json")
NODE_NAME="${NODE_VERSION}-linux-${NODE_ARCH}"
NODE_ARCHIVE="node-${NODE_NAME}.tar.xz"
if [[ ! -x "$APP_ROOT/toolchains/node-$NODE_NAME/bin/node" ]]; then
  fetch "https://nodejs.org/dist/$NODE_VERSION/SHASUMS256.txt" -o "$TOOL_TMP/node.sha256"
  fetch "https://nodejs.org/dist/$NODE_VERSION/$NODE_ARCHIVE" -o "$TOOL_TMP/node.tar.xz"
  NODE_SHA=$(awk -v name="$NODE_ARCHIVE" '$2 == name {print $1}' "$TOOL_TMP/node.sha256")
  python3 "$HELPER" verify --file "$TOOL_TMP/node.tar.xz" --sha256 "$NODE_SHA"
  tar -xJf "$TOOL_TMP/node.tar.xz" -C "$TOOL_TMP"
  mv "$TOOL_TMP/node-$NODE_NAME" "$APP_ROOT/toolchains/node-$NODE_NAME"
fi
rm -rf -- "$TOOL_TMP"
BUILD_PATH="$APP_ROOT/toolchains/$GO_VERSION/bin:$APP_ROOT/toolchains/node-$NODE_NAME/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
chmod -R a+rX "$APP_ROOT/toolchains/$GO_VERSION" "$APP_ROOT/toolchains/node-$NODE_NAME"

STEP=构建前后端
RELEASE_ID="$(date -u +%Y%m%dT%H%M%SZ)-$(openssl rand -hex 3)"
BUILD_DIR="$APP_ROOT/builds/$RELEASE_ID"
RELEASE_DIR="$APP_ROOT/releases/$RELEASE_ID"
install -d -m 755 "$BUILD_DIR/backend" "$BUILD_DIR/frontend" "$RELEASE_DIR/bin" "$RELEASE_DIR/frontend"
rsync -a --exclude='bin/' --exclude='data/' --exclude='uploads/' --exclude='.env*' "$SOURCE_DIR/backend/" "$BUILD_DIR/backend/"
rsync -a --exclude='node_modules/' --exclude='dist/' --exclude='.env*' --exclude='*.tsbuildinfo' "$SOURCE_DIR/frontend/" "$BUILD_DIR/frontend/"
chown -R wybo:wybo "$BUILD_DIR"
chmod 755 "$BUILD_DIR"
# Positional parameters are intentionally expanded by the unprivileged child shell.
# shellcheck disable=SC2016
runuser -u wybo -- env PATH="$BUILD_PATH" GOCACHE=/var/cache/wybo/go-build GOMODCACHE=/var/cache/wybo/go-mod \
  bash -c 'set -e; cd "$1"; go test ./...; CGO_ENABLED=0 go build -trimpath -o "$2" ./cmd/server; CGO_ENABLED=0 go build -trimpath -o "$3" ./cmd/admin-recover' \
  bash "$BUILD_DIR/backend" "$BUILD_DIR/wybo-server" "$BUILD_DIR/admin-recover"
# shellcheck disable=SC2016
runuser -u wybo -- env PATH="$BUILD_PATH" npm_config_cache=/var/cache/wybo/npm VITE_API_BASE_URL=/api/v1 VITE_DEMO_MODE=false \
  bash -c 'set -e; cd "$1"; npm ci --no-audit --no-fund; npm run build' bash "$BUILD_DIR/frontend"
install -m 755 "$BUILD_DIR/wybo-server" "$RELEASE_DIR/bin/wybo-server"
install -m 755 "$BUILD_DIR/admin-recover" "$RELEASE_DIR/bin/admin-recover"
cp -a "$BUILD_DIR/frontend/dist/." "$RELEASE_DIR/frontend/"
chown -R root:root "$RELEASE_DIR"
chmod -R a+rX "$RELEASE_DIR"

STEP=配置数据库
python3 "$HELPER" database-sql --state "$STATE" | runuser -u postgres -- psql -Xw --host=/var/run/postgresql --port=5432 --username=postgres --set ON_ERROR_STOP=1 --dbname postgres
DB_PASSWORD=$(get_state db_password)
PGPASSWORD="$DB_PASSWORD" psql -Xw -h 127.0.0.1 -p 5432 -U wybo --dbname wybo --set ON_ERROR_STOP=1 -c 'SELECT 1' >/dev/null
unset DB_PASSWORD

STEP=生成服务配置
RENDER_DIR=$(mktemp -d /etc/wybo/.render.XXXXXXXX)
python3 "$HELPER" render --state "$STATE" --output "$RENDER_DIR"
if [[ -L "$APP_ROOT/current" ]]; then
  /usr/local/sbin/wybo-backup
  readlink -f "$APP_ROOT/current" > /etc/wybo/previous-release
fi
install -m 600 "$RENDER_DIR/wybo.env" /etc/wybo/wybo.env
install -m 640 -o root -g wybo "$RENDER_DIR/config.yaml" /etc/wybo/config.yaml
install -m 600 "$RENDER_DIR/credentials.txt" /etc/wybo/credentials.txt
install -m 644 "$RENDER_DIR/wybo.service" /etc/systemd/system/wybo.service
install -m 644 "$RENDER_DIR/wybo-backup.service" /etc/systemd/system/wybo-backup.service
install -m 644 "$RENDER_DIR/wybo-backup.timer" /etc/systemd/system/wybo-backup.timer
install -m 700 "$SCRIPT_DIR/deploy/backup.sh" /usr/local/sbin/wybo-backup
systemctl stop wybo 2>/dev/null || true
ln -s "$RELEASE_DIR" "$APP_ROOT/.current-$RELEASE_ID"
mv -Tf "$APP_ROOT/.current-$RELEASE_ID" "$APP_ROOT/current"
systemctl daemon-reload
systemctl enable --now wybo
python3 "$HELPER" wait-http --url http://127.0.0.1:8080/healthz --seconds 90
systemctl is-active --quiet wybo
python3 "$HELPER" site-sql --state "$STATE" | runuser -u postgres -- psql -Xw --host=/var/run/postgresql --port=5432 --username=postgres --set ON_ERROR_STOP=1 --dbname wybo

STEP=配置域名和申请SSL
if [[ -s "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" && -s "/etc/letsencrypt/live/$DOMAIN/privkey.pem" ]]; then
  install -m 644 "$RENDER_DIR/nginx-https.conf" /etc/nginx/sites-available/wybo.conf
else
  install -m 644 "$RENDER_DIR/nginx-http.conf" /etc/nginx/sites-available/wybo.conf
fi
ln -sfn /etc/nginx/sites-available/wybo.conf /etc/nginx/sites-enabled/wybo.conf
nginx -t
systemctl reload nginx
if command -v ufw >/dev/null && LC_ALL=C ufw status | grep '^Status: active$' >/dev/null; then
  ufw allow 80/tcp
  ufw allow 443/tcp
fi
certbot certonly --webroot -w /var/www/letsencrypt --domain "$DOMAIN" --cert-name "$DOMAIN" \
  --email "$EMAIL" --agree-tos --non-interactive --keep-until-expiring
install -m 644 "$RENDER_DIR/nginx-https.conf" /etc/nginx/sites-available/wybo.conf
nginx -t
systemctl reload nginx
install -d -m 755 /etc/letsencrypt/renewal-hooks/deploy
install -m 755 "$RENDER_DIR/cert-renew-hook" /etc/letsencrypt/renewal-hooks/deploy/wybo-nginx
systemctl enable --now certbot.timer wybo-backup.timer
curl --fail --silent --show-error --connect-timeout 5 --max-time 15 --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/healthz" >/dev/null
/usr/local/sbin/wybo-backup
rm -rf -- "$RENDER_DIR" "$BUILD_DIR"

log "部署完成：https://$DOMAIN"
printf '后台：https://%s/admin（已修改过入口时使用原入口）\n' "$DOMAIN"
printf '凭据文件：/etc/wybo/credentials.txt（root 可读，密码不打印到日志）\n'
printf '2FA：登录后台 → 安全设置，自行扫码并确认启用。\n'
printf '支付指引：项目 docs/支付渠道配置指南.md\n'
printf '日志：journalctl -u wybo -f；备份：sudo wybo-backup\n'
