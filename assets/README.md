# Wybo 品牌资源（望月AI代充小铺）

望月主题：夜空 + 满月 + 望月小兔子 + 星星。

## 文件说明

| 文件 | 说明 |
|------|------|
| `logo-icon.svg` | 方形图标（源文件），用于生成 favicon / PNG |
| `logo.svg` | 横版文字标（图标 + Wybo + 望月AI代充小铺） |
| `favicon.ico` | 多尺寸 ICO（16/32/48/64/128/256） |
| `png/logo-*.png` | 各尺寸 PNG（含 180 = apple-touch-icon） |

## 配色

- 夜空蓝紫 `#3D3A8C`（主）
- 奶油月亮黄 `#FFD166`
- 樱粉 `#FF9EC4`
- 星光浅蓝 `#A8D8EA`

## 重新生成 PNG / ICO

```bash
cd tools/assets
npm install            # 安装 sharp（SVG→PNG 渲染）
node render.mjs        # 由 logo-icon.svg 生成 png/logo-*.png
python3 make_ico.py    # 由 PNG 组装 favicon.ico（需 Pillow）
```

生成后同步到前端：

```bash
cp assets/logo-icon.svg frontend/public/logo-icon.svg
cp assets/logo.svg frontend/public/logo.svg
cp assets/favicon.ico frontend/public/favicon.ico
cp assets/png/logo-180.png frontend/public/apple-touch-icon.png
```
