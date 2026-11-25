# 青屿集 Qingyu 品牌资源

默认站标是一只戴珊瑚橘渔夫帽的海獭。店主上传的 Logo 和浏览器图标优先展示。

| 文件 | 用途 |
|------|------|
| `logo-icon.svg` | 默认站标，也是各尺寸 PNG 和 favicon 的源图 |
| `logo.svg` | 带「青屿集」和 Qingyu 的横版标志 |
| `png/logo-*.png` | 16 到 512 像素的站标，180 像素用于 Apple 触屏图标 |
| `favicon.ico` | 包含 16、32、48、64、128 和 256 像素图标 |

`tools/assets/render.mjs` 渲染 PNG 并将 SVG 和触屏图标同步到前端。`tools/assets/make_ico.py` 使用 Pillow 生成前端与品牌目录中的 ICO。

## 商店明信片

首页海獭小店插画使用内置 ImageGen 生成。网页使用 1134 × 756 的 WebP，店名、说明和按钮由页面排版。 [商店插画](../frontend/public/images/island-shop.webp)

<details>
<summary>查看生成提示词</summary>

> Create a professionally art-directed original illustration for the illustrated banner of a playful Chinese island digital shop. Landscape 3:2 aspect ratio, edge-to-edge single scene. Graphic two-dimensional hand-inked cartoon, a lovable confident sea otter with stubby ears, chocolate-brown fur, tan rounded muzzle, little dark oval nose, a warm red-orange fishing bucket hat and teal neckerchief. The hero otter leans out a small wooden floating kiosk with striped coral and sun-yellow awning, one paw waving, the other on the counter. Feature the character in the middle-right at a large readable scale, kiosk suspended on a rocky postage-stamp island with two friendly waves curling around it. A perched small cream seagull, two small round clouds and a few whimsical little ocean splashes, no busy background. Intentional bold slightly wobbly dark teal ink outlines, varied thick weight, warm flat spot colours of yellow ochre, pale butter cream, burnt coral, seafoam turquoise, teal and dark chocolate, joyful illustrated stationery or screen-printed Japanese seaside postcard, charming beautiful crafted shapes, limited dappled grain and simple shaded blocks. The FAR LEFT edge and top corners are calm pale butter #fff0bc sky with ample air, waves and island concentrated around the kiosk. Unframed bleed, no letters, no signs with text, no badges, no UI, no watermarks, no photorealism, no faux 3D, no gradients. The kiosk and otter hat must be fully visible and the whole vignette balanced as standalone display art.

</details>
