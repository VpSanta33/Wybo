#!/usr/bin/env bash
# Installed as /usr/local/sbin/wybo-backup. PostgreSQL uses the local OS peer account.
set +x
set -Eeuo pipefail
umask 077
[[ $EUID == 0 ]] || { printf '请使用 sudo wybo-backup\n' >&2; exit 1; }
[[ -f /etc/wybo/deployment.json ]] || { printf '缺少 Wybo 部署配置\n' >&2; exit 1; }
install -d -m 700 /var/backups/wybo
exec 9>/var/backups/wybo/.backup.lock
flock -n 9 || { printf '另一个备份任务正在运行\n' >&2; exit 1; }
BACKUP_TMP=$(mktemp -d /var/backups/wybo/.partial.XXXXXXXX)
trap 'rm -rf -- "$BACKUP_TMP"' EXIT
runuser -u postgres -- pg_dump --no-password --host=/var/run/postgresql --port=5432 --username=postgres --format=custom --no-owner --no-acl --dbname wybo > "$BACKUP_TMP/database.dump"
runuser -u postgres -- pg_restore --list < "$BACKUP_TMP/database.dump" > "$BACKUP_TMP/database-toc.txt"
tar --exclude='wybo/.render.*' -czf "$BACKUP_TMP/config.tar.gz" -C /etc wybo
tar -czf "$BACKUP_TMP/uploads.tar.gz" -C /var/lib/wybo uploads
readlink -f /opt/wybo/current > "$BACKUP_TMP/release.txt"
(cd "$BACKUP_TMP" && sha256sum database.dump config.tar.gz uploads.tar.gz > SHA256SUMS)
printf 'Wybo native backup v1\n' > "$BACKUP_TMP/COMPLETE"
chmod -R go-rwx "$BACKUP_TMP"
BACKUP_DIR="/var/backups/wybo/$(date -u +%Y%m%dT%H%M%SZ)-${BACKUP_TMP##*.}"
mv "$BACKUP_TMP" "$BACKUP_DIR"
trap - EXIT
printf '备份完成：%s\n' "$BACKUP_DIR"
