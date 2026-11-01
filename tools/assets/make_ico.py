#!/usr/bin/env python3
"""由 assets/png/logo-*.png 组装多尺寸 favicon.ico（需 Pillow）。"""
import os
from PIL import Image

root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
png_dir = os.path.join(root, "assets", "png")
out_ico = os.path.join(root, "assets", "favicon.ico")

sizes = [16, 32, 48, 64, 128, 256]
images = [Image.open(os.path.join(png_dir, f"logo-{s}.png")) for s in sizes]

# 以最大尺寸为基底，Pillow 会按 sizes 内嵌各尺寸
images[-1].save(out_ico, format="ICO", sizes=[(s, s) for s in sizes])

# 同步 favicon.ico 到前端 public
pub = os.path.join(root, "frontend", "public")
os.makedirs(pub, exist_ok=True)
with open(out_ico, "rb") as f:
    data = f.read()
with open(os.path.join(pub, "favicon.ico"), "wb") as f:
    f.write(data)

print(f"✓ favicon.ico 已生成（{sizes}），并同步到 frontend/public")
