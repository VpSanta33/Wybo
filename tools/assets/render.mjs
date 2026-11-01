import sharp from 'sharp'
import { mkdir, readFile, copyFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const root = join(__dirname, '..', '..')
const src = join(root, 'assets', 'logo-icon.svg')
const outDir = join(root, 'assets', 'png')

// 各尺寸（180 用作 apple-touch-icon）
const sizes = [16, 32, 48, 64, 128, 180, 192, 256, 512]

await mkdir(outDir, { recursive: true })
const svgBuffer = await readFile(src)

for (const s of sizes) {
  await sharp(svgBuffer).resize(s, s).png().toFile(join(outDir, `logo-${s}.png`))
  console.log(`✓ logo-${s}.png`)
}

// 同步 SVG 与 apple-touch-icon 到前端 public
const pub = join(root, 'frontend', 'public')
await mkdir(pub, { recursive: true })
await copyFile(join(root, 'assets', 'logo-icon.svg'), join(pub, 'logo-icon.svg'))
await copyFile(join(root, 'assets', 'logo.svg'), join(pub, 'logo.svg'))
await copyFile(join(outDir, 'logo-180.png'), join(pub, 'apple-touch-icon.png'))
console.log('✓ 已同步 logo-icon.svg / logo.svg / apple-touch-icon.png 到 frontend/public')

console.log('done')
