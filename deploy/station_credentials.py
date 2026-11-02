#!/usr/bin/env python3
"""Validate/copy Certbot credentials without printing a DNS secret."""
import configparser
import os
from pathlib import Path
import re
import stat
import sys

def prepare(source, destination=None):
    source = Path(source)
    info = source.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077:
        raise ValueError("Cloudflare 凭据须为权限 600 的普通文件，不能是符号链接")
    text = source.read_text()
    parser = configparser.ConfigParser(interpolation=None)
    parser.read_string("[dns]\n" + text)
    if set(parser["dns"]) != {"dns_cloudflare_api_token"} or not re.fullmatch(r"[A-Za-z0-9_-]{20,256}",parser["dns"]["dns_cloudflare_api_token"]):
        raise ValueError("文件应只包含 dns_cloudflare_api_token = 你的 API Token")
    if destination:
        target=Path(destination)
        if target.is_symlink():
            raise ValueError("Cloudflare 凭据目标不能是符号链接")
        if source.resolve() != target.resolve():
            fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_TRUNC|os.O_NOFOLLOW,0o600)
            with os.fdopen(fd,"w") as stream:
                stream.write(text)
        target.chmod(0o600)

if __name__ == "__main__":
    try:
        prepare(*sys.argv[1:])
    except (ValueError, OSError, configparser.Error):
        sys.exit("Cloudflare 凭据无效：请检查文件格式、权限 600 及 DNS API Token；未输出密钥。")
