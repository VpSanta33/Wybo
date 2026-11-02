#!/usr/bin/env python3
"""Native deployment config generator. Standard library only; never source secrets as shell code."""

import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import socket
import stat
import sys
import time
import urllib.error
import urllib.request


def validate(domain, email, admin_user):
    if not isinstance(domain, str) or len(domain) > 253 or domain != domain.lower():
        raise ValueError("域名需要小写 ASCII；中文域名请先转为 Punycode")
    labels = domain.split(".")
    label_pattern = r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?"
    if len(labels) < 2 or not all(re.fullmatch(label_pattern, x) for x in labels):
        raise ValueError("域名格式无效，不要填写协议、端口、路径或通配符")
    try:
        ipaddress.ip_address(domain)
    except ValueError:
        pass
    else:
        raise ValueError("请提供域名，不能使用 IP 地址申请此部署证书")
    if labels[-1].isdigit() or labels[-1] in ("local", "localhost", "internal"):
        raise ValueError("请使用可解析的公网域名")
    if not re.fullmatch(r"[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?\.[A-Za-z]{2,63}", email) or len(email) > 254:
        raise ValueError("证书联系邮箱无效")
    if not re.fullmatch(r"[A-Za-z0-9_]{3,32}", admin_user):
        raise ValueError("管理员用户名应为 3–32 位字母、数字或下划线")


def load_state(path):
    path = Path(path)
    if path.is_symlink() or not stat.S_ISREG(path.stat().st_mode) or path.stat().st_mode & 0o077:
        raise ValueError("部署凭据文件必须是普通文件且权限为 600")
    state = json.loads(path.read_text())
    if state.get("version") != 1:
        raise ValueError("不支持的部署配置版本")
    validate(state["domain"], state["email"], state["admin_user"])
    for key, length in (("db_password", 64), ("jwt_secret", 96), ("admin_password", 32)):
        if not re.fullmatch(r"[0-9a-f]{%d}" % length, state.get(key, "")):
            raise ValueError("部署凭据损坏：" + key)
    root = state.get("station_root", "")
    if root:
        validate(root, state["email"], state["admin_user"])
        if state["domain"] != root and not (state["domain"].endswith("." + root) and state["domain"].count(".") == root.count(".") + 1):
            raise ValueError("分站根域名须为主站域名或主站的直接父域名")
    state.setdefault("station_root", "")
    return state


def init_state(path, domain, email, admin_user):
    validate(domain, email, admin_user)
    path = Path(path)
    if path.exists():
        state = load_state(path)
        if (domain, email, admin_user) != (state["domain"], state["email"], state["admin_user"]):
            raise ValueError("已有部署配置不同，拒绝覆盖凭据")
        return state
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    state = dict(version=1, station_root="", domain=domain, email=email, admin_user=admin_user,
                 db_password=secrets.token_hex(32), jwt_secret=secrets.token_hex(48),
                 admin_password=secrets.token_hex(16))
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as stream:
        json.dump(state, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    return state


HTTP_TEMPLATE = """# Managed by Wybo native deploy.
server {
    listen 80;
    listen [::]:80;
    server_name @DOMAIN@;
    location ^~ /.well-known/acme-challenge/ { root /var/www/letsencrypt; }
    location / { return 503; }
}
"""

HTTPS_TEMPLATE = """# Managed by Wybo native deploy. DNS must point directly to this server.
server {
    listen 80;
    listen [::]:80;
    server_name @DOMAIN@;
    location ^~ /.well-known/acme-challenge/ { root /var/www/letsencrypt; }
    location / { return 308 https://@DOMAIN@$request_uri; }
}
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name @DOMAIN@;
    ssl_certificate /etc/letsencrypt/live/@DOMAIN@/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/@DOMAIN@/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:WyboSSL:10m;
    ssl_session_timeout 1d;
    server_tokens off;
    root /opt/wybo/current/frontend;
    index index.html;
    client_max_body_size 6m;
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options DENY always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;
    # No access log: order lookups contain customer contacts in query parameters.
    access_log off;
    error_log /var/log/nginx/wybo-error.log warn;
    location ^~ /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
        proxy_send_timeout 60s;
        add_header Cache-Control "no-store" always;
    }
    location ^~ /uploads/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    location = /healthz { proxy_pass http://127.0.0.1:8080; }
    location /assets/ {
        try_files $uri =404;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
    location = /index.html { add_header Cache-Control "no-store"; }
    location ~ /\\. { deny all; }
    location / { try_files $uri $uri/ /index.html; }
}
"""

SYSTEMD_SERVICE = """[Unit]
Description=Wybo storefront backend
Wants=network-online.target
After=network-online.target postgresql.service
Requires=postgresql.service
StartLimitIntervalSec=120
StartLimitBurst=5

[Service]
Type=simple
User=wybo
Group=wybo
WorkingDirectory=/etc/wybo
EnvironmentFile=/etc/wybo/wybo.env
ExecStart=/opt/wybo/current/bin/wybo-server
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/wybo /var/cache/wybo
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
StandardOutput=journal
StandardError=journal
SyslogIdentifier=wybo

[Install]
WantedBy=multi-user.target
"""


def render(state, output):
    out = Path(output)
    out.mkdir(parents=True, exist_ok=True, mode=0o700)
    domain = state["domain"]
    url = "https://" + domain
    config = {
        "server": {"public_host": domain,"station_root":state.get("station_root", ""),"addr": "127.0.0.1:8080", "mode": "release", "trusted_proxies": ["127.0.0.1", "::1"]},
        "db": {"driver": "postgres"}, "jwt": {"expire_hours": 24},
        "pay": {"return_url": url + "/orders", "notify_base": url + "/api/v1/pay/notify"},
        "upload": {"dir": "/var/lib/wybo/uploads", "max_size": 5 * 1024 * 1024},
    }
    # JSON is also valid YAML; values are quoted by a real serializer.
    env = {
        "TZ": "Asia/Shanghai", "SERVER_ADDR": "127.0.0.1:8080", "SERVER_MODE": "release",
        "SERVER_PUBLIC_HOST":domain,"SERVER_STATION_ROOT":state.get("station_root", ""),
        "DB_DRIVER": "postgres",
        "DB_DSN": f"host=127.0.0.1 port=5432 user=wybo password={state['db_password']} dbname=wybo sslmode=disable TimeZone=Asia/Shanghai",
        "JWT_SECRET": state["jwt_secret"], "ADMIN_USERNAME": state["admin_user"],
        "ADMIN_PASSWORD": state["admin_password"], "UPLOAD_DIR": "/var/lib/wybo/uploads",
        "PAY_RETURN_URL": url + "/orders", "PAY_NOTIFY_BASE": url + "/api/v1/pay/notify",
    }
    credentials = f"""Wybo 原生部署凭据（仅 root 可读）
站点：{url}
首次后台入口：{url}/admin
初始管理员：{state['admin_user']}
初始密码：{state['admin_password']}
数据库：127.0.0.1:5432 / wybo
数据库用户：wybo（非超级用户）
数据库密码：{state['db_password']}

管理员密码只用于首次初始化。在后台改密后，以新密码为准。
2FA 由管理员在后台「安全设置」自行开启；部署或升级不改动已有 2FA。
JWT_SECRET 保存在 /etc/wybo/wybo.env 和 deployment.json；与数据库一起备份。
该密钥也用于加密支付、邮件与 TOTP 配置，不可直接重新生成或替换。
"""
    server_names = domain + (" *." + state["station_root"] if state.get("station_root") else "")
    https = HTTPS_TEMPLATE.replace("@DOMAIN@",domain).replace("server_name " + domain + ";", "server_name " + server_names + ";")
    # Preserve the matched shop hostname instead of redirecting every shop to the main site.
    if state.get("station_root"):
        https = https.replace("return 308 https://" + domain + "$request_uri;", "return 308 https://$host$request_uri;")
    files = {
        "config.yaml": (json.dumps(config, ensure_ascii=False, indent=2) + "\n", 0o640),
        "wybo.env": ("".join(f'{key}="{value}"\n' for key, value in env.items()), 0o600),
        "credentials.txt": (credentials, 0o600),
        "nginx-http.conf": (HTTP_TEMPLATE.replace("@DOMAIN@", domain), 0o644),
        "nginx-https.conf": (https, 0o644),
        "wybo.service": (SYSTEMD_SERVICE, 0o644),
        "cert-renew-hook": ("#!/bin/sh\nset -eu\n/usr/sbin/nginx -t\n/bin/systemctl reload nginx\n", 0o755),
        "wybo-backup.service": ("[Unit]\nDescription=Wybo PostgreSQL and configuration backup\nAfter=postgresql.service\n\n[Service]\nType=oneshot\nUMask=0077\nExecStart=/usr/local/sbin/wybo-backup\n", 0o644),
        "wybo-backup.timer": ("[Unit]\nDescription=Daily Wybo backup\n\n[Timer]\nOnCalendar=*-*-* 03:20:00 Asia/Shanghai\nRandomizedDelaySec=15m\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n", 0o644),
    }
    for name, (content, mode) in files.items():
        dest = out / name
        if dest.is_symlink():
            raise ValueError("输出位置不能是符号链接：" + name)
        dest.write_text(content)
        dest.chmod(mode)


def versions(kind, data, go_mod=None):
    if kind == "node":
        candidates = [x["version"] for x in data if re.fullmatch(r"v22\.\d+\.\d+", x["version"]) and x.get("lts")]
        if not candidates:
            raise ValueError("官方元数据中没有 Node 22 LTS")
        return max(candidates, key=lambda v: tuple(map(int, v[1:].split("."))))
    match = re.search(r"(?m)^go (\d+)\.(\d+)\.(\d+)\s*$", Path(go_mod).read_text())
    if not match:
        raise ValueError("go.mod 缺少明确 Go 工具链版本")
    minimum = tuple(map(int, match.groups()))
    candidates = []
    for item in data:
        m = re.fullmatch(r"go(\d+)\.(\d+)\.(\d+)", item["version"])
        if m and item.get("stable"):
            version = tuple(map(int, m.groups()))
            if version[:2] == minimum[:2] and version >= minimum:
                candidates.append((version, item["version"]))
    if not candidates:
        raise ValueError("官方元数据中没有满足 go.mod 的稳定 Go 版本")
    return max(candidates)[1]


def database_sql(state):
    # All interpolated secrets are validated hexadecimal; names are fixed.
    password = state["db_password"]
    return f"""SELECT format('CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE', 'wybo', '{password}')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='wybo')\n\\gexec
SELECT 'CREATE DATABASE wybo OWNER wybo ENCODING ''UTF8'' TEMPLATE template0'
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='wybo')\n\\gexec
REVOKE CONNECT ON DATABASE wybo FROM PUBLIC;
GRANT CONNECT ON DATABASE wybo TO wybo;
"""


def site_sql(state):
    url = "https://" + state["domain"]
    return "\n".join(
        f"INSERT INTO settings (key,value,updated_at) VALUES ('{k}','{v}',NOW()) ON CONFLICT (key) DO NOTHING;"
        for k, v in (("site_url", url), ("pay_notify_base", url + "/api/v1/pay/notify"), ("pay_return_url", url + "/orders"))
    )


def check_postgres(listen):
    # Do not silently reconfigure a pre-existing PostgreSQL installation.
    if any(x.strip() not in ("localhost", "127.0.0.1", "::1") for x in listen.split(",")):
        raise ValueError("PostgreSQL 当前允许非本机连接。请先核对现有业务并将 listen_addresses 配置为 localhost，再继续部署")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    for command in ("validate", "init"):
        p = sub.add_parser(command)
        for name in ("domain", "email", "admin-user"):
            p.add_argument("--" + name, required=True)
        if command == "validate":
            p.add_argument("--station-root",default="")
        if command == "init":
            p.add_argument("--state", required=True)
    for command in ("get", "render", "database-sql", "site-sql"):
        p = sub.add_parser(command)
        p.add_argument("--state", required=True)
        if command == "get":
            p.add_argument("--key", required=True)
        if command == "render":
            p.add_argument("--output", required=True)
    p = sub.add_parser("stations")
    p.add_argument("--state",required=True)
    p.add_argument("--root",required=True)
    p = sub.add_parser("version")
    p.add_argument("--kind", choices=("node", "go"), required=True)
    p.add_argument("--metadata", required=True)
    p.add_argument("--go-mod")
    p = sub.add_parser("checksum")
    p.add_argument("--metadata", required=True)
    p.add_argument("--filename", required=True)
    p = sub.add_parser("verify")
    p.add_argument("--file", required=True)
    p.add_argument("--sha256", required=True)
    p = sub.add_parser("resolve")
    p.add_argument("--domain", required=True)
    p = sub.add_parser("check-port")
    p.add_argument("--port", type=int, required=True)
    p = sub.add_parser("check-postgres")
    p.add_argument("--listen", required=True)
    p = sub.add_parser("wait-http")
    p.add_argument("--url", required=True)
    p.add_argument("--seconds", type=int, default=90)
    args = parser.parse_args()
    if args.command == "stations":
        state = load_state(args.state)
        validate(args.root,state["email"],state["admin_user"])
        if state.get("station_root") and state["station_root"] != args.root:
            raise ValueError("已有分站根域名不同，迁移请先按文档安排，禁止覆盖")
        if state["domain"] != args.root and not (state["domain"].endswith("."+args.root) and state["domain"].count(".") == args.root.count(".")+1):
            raise ValueError("分站根域名须为主站域名或主站的直接父域名")
        state["station_root"] = args.root
        path = Path(args.state)
        temporary = path.with_name(path.name + ".stations.tmp")
        fd=os.open(temporary,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        with os.fdopen(fd,"w") as stream:
            json.dump(state,stream,ensure_ascii=False,indent=2)
            stream.write("\n")
        os.replace(temporary,path)
    elif args.command in ("validate", "init"):
        validate(args.domain, args.email, args.admin_user)
        if args.command == "validate" and args.station_root:
            validate(args.station_root,args.email,args.admin_user)
            if args.domain != args.station_root and not (args.domain.endswith("."+args.station_root) and args.domain.count(".") == args.station_root.count(".")+1):
                raise ValueError("分站根域名须为主站域名或主站的直接父域名")
        if args.command == "init":
            init_state(args.state, args.domain, args.email, args.admin_user)
    elif args.command in ("get", "render", "database-sql", "site-sql"):
        state = load_state(args.state)
        if args.command == "get":
            print(state[args.key])
        elif args.command == "render":
            render(state, args.output)
        elif args.command == "database-sql":
            print(database_sql(state))
        else:
            print(site_sql(state))
    elif args.command == "version":
        print(versions(args.kind, json.loads(Path(args.metadata).read_text()), args.go_mod))
    elif args.command == "checksum":
        data = json.loads(Path(args.metadata).read_text())
        matches = [f["sha256"] for v in data for f in v["files"] if f["filename"] == args.filename]
        if len(matches) != 1:
            raise ValueError("官方元数据中找不到唯一的工具链校验值")
        print(matches[0])
    elif args.command == "verify":
        if not re.fullmatch(r"[0-9a-f]{64}", args.sha256):
            raise ValueError("官方 SHA-256 校验值无效")
        digest = hashlib.sha256()
        with open(args.file, "rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                digest.update(chunk)
        if not secrets.compare_digest(digest.hexdigest(), args.sha256):
            raise ValueError("工具链下载校验失败")
    elif args.command == "resolve":
        addresses = sorted({x[4][0] for x in socket.getaddrinfo(args.domain, 80, type=socket.SOCK_STREAM)})
        if not addresses or any(not ipaddress.ip_address(x).is_global for x in addresses):
            raise ValueError("域名必须解析到公网 IP；请检查 DNS 的 A / AAAA 记录")
        print("DNS 解析：" + ", ".join(addresses))
    elif args.command == "check-postgres":
        check_postgres(args.listen)
    elif args.command == "check-port":
        with socket.socket() as sock:
            try:
                sock.bind(("127.0.0.1", args.port))
            except OSError as e:
                raise ValueError(f"本机端口 {args.port} 已占用，请先核对现有服务") from e
    elif args.command == "wait-http":
        deadline = time.monotonic() + args.seconds
        while time.monotonic() < deadline:
            try:
                with urllib.request.urlopen(args.url, timeout=3) as response:
                    data = json.load(response)
                    if data.get("code") == 0 and data.get("data", {}).get("status") == "up":
                        return
            except (OSError, ValueError, urllib.error.URLError):
                pass
            time.sleep(1)
        raise ValueError("后端未能正常启动，请查看 journalctl -u wybo -n 80")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError) as error:
        sys.exit("[Wybo] " + str(error))
