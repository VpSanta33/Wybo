# Wybo

Wybo 是一个面向数字商品、卡密和 AI 会员服务的销售平台，包含顾客商城、后台管理和代理中心。支持自动发卡与手工发货，并提供支付接入、成本利润统计、订单查询和邮件通知。

前台采用望月与小兔子主题，网站名称、Logo、商品图片、支付方式对外名称及兑换地址均可在后台配置。

## 功能

| 模块 | 功能 |
| --- | --- |
| 商品与分类 | 分类管理、商品上下架、成本与售价设置、商品图片上传、自动或手工发货 |
| 库存与采购 | 卡密批量导入、去重、库存批次、采购记录、供应商资料及低库存提醒 |
| 订单与交付 | 库存预占、超时关闭、支付回调验签、自动发卡、手工发货及订单查询 |
| 支付管理 | Stripe、易支付、DujiaoPay、BEpusdt，支持设置对外名称与上传 Logo |
| 经营报表 | 销售额、商品成本、销售利润、经营净收益、访问流量、转化与 CSV 导出 |
| 营销与售后 | 优惠券、公告、售后工单、人工补卡、升级补差与退款处理 |
| 代理分站 | 独立子域名店铺与后台、申请审核、独立品牌、商品授权、供货差价与结算 |
| 站点互联 | Wybo 站点间商品同步、上游采购、下游钱包与授权、报价下单、交付回调及资金流水 |
| 分销代理 | 代理资料、专属价格、推广链接、订单归属、佣金与结算记录 |
| 通知与推送 | Bark 手机成交通知、Resend 发货邮件、成交 Webhook、运营提醒与失败重试 |
| 站点与安全 | 品牌设置、兑换说明、自定义后台入口、密码修改、可选 2FA、角色权限、操作审计与 Cloudflare Turnstile |
| 运维 | 上线检查、运行监控、备份与恢复工具、原生部署及更新脚本 |

### 发货与利润

自动发货商品在下单时预占库存，确认收款后发放卡密；订单取消或超时后释放库存。手工发货商品在收款后进入待发货状态，由管理员填写交付内容，顾客可在订单页查看，并接收已配置的邮件通知。

订单保存下单时的成本和发货方式，后续修改商品不会改写历史订单。例如成本为 105 元、售价为 115 元，销售利润为 10 元。经营净收益还需计入退款、渠道手续费和代理佣金等项目，未录入的费用不会直接视为零。

## 技术栈

| 部分 | 技术 |
| --- | --- |
| 前端 | React 18、TypeScript、Vite 6、React Router、Recharts |
| 后端 | Go 1.25、Gin、GORM、Viper、Zap、JWT |
| 数据库 | SQLite 用于本地开发，PostgreSQL 用于原生生产部署 |
| 生产环境 | Nginx、systemd、Certbot、Node.js 22 LTS |

## 生产部署

项目使用原生部署脚本，不依赖 Docker。支持 Ubuntu 22.04 / 24.04、Debian 12 / 13，支持 amd64 和 arm64，需要 root 或 sudo 权限及 systemd。

先将域名解析到服务器，并放行 TCP 80、443 端口。建议至少 2 核、2 GB 内存和 10 GB 可用磁盘，构建阶段建议 4 GB 内存。

```bash
git clone https://github.com/VpSanta33/Wybo.git
cd Wybo
sudo bash deploy.sh install \
  --domain shop.example.com \
  --email ops@example.com
```

将示例域名和邮箱替换为自己的。脚本会安装 PostgreSQL、Nginx 和 Certbot，下载独立 Go / Node 工具链，执行测试与构建，生成随机数据库密码、初始管理员密码及配置加密密钥，并配置 HTTPS、证书续期、systemd 服务与每日备份。

安装后在服务器查看初始凭据：

```bash
sudo cat /etc/wybo/credentials.txt
```

默认后台入口为 `https://shop.example.com/admin`。登录后可修改密码及后台访问地址；2FA 由用户在后台安全设置中自行绑定，部署脚本不会自动开启。

首次部署会创建新的 PostgreSQL 数据库，不会自动迁移本地 SQLite 数据。完整步骤、文件位置与恢复方法见 [原生部署指南](docs/原生部署.md)。

分站模式的 DNS、泛域名 HTTPS、申请审核和结算操作见 [代理分站指南](docs/代理分站.md)。已有部署可通过更新脚本启用，保留原有数据库和凭据。

## 更新版本

进入服务器上最初克隆的源码目录，拉取代码并执行更新：

```bash
cd /root/Wybo
git pull --ff-only origin main
sudo bash deploy.sh update
```

如果项目位于其他路径，请替换 `/root/Wybo`。更新先测试和构建新版本，再备份并切换运行版本，重启时会有短暂中断。已有数据库、后台配置、上传文件、管理员密码和 2FA 绑定会保留。

日常维护命令：

```bash
# 查看运行状态
sudo bash deploy.sh status

# 创建备份
sudo bash deploy.sh backup

# 查看应用日志
sudo journalctl -u wybo -f
```

`JWT_SECRET` 同时用于保护支付、邮件和 2FA 配置，需保持稳定，并与数据库一同备份。不要删除 `/etc/wybo/deployment.json` 或随意重新生成密钥。恢复流程见 [原生部署指南](docs/原生部署.md)。

## 首次配置

1. 在安全设置中修改密码，按需修改后台入口并开启 2FA。
2. 在站点设置中填写网站名称、上传 Logo，设置正式域名、兑换地址及兑换说明。
3. 创建分类与商品，填写成本、售价和发货方式，上传商品图片；自动发货商品需要导入卡密。
4. 在支付渠道中填写商户配置、设置对外名称与 Logo，核对回调地址后启用。
5. 在邮件设置中配置 Resend API Key、已验证的发信域名和通知收件人。
6. 按需配置 Cloudflare Turnstile，并完成一次下单、支付、发货及订单查询验证。

商品价格为 0 时显示待定价，不能购买。删除商品前需要处理关联数据；已有订单、库存或采购等记录的商品应使用下架。分类下仍有商品时不能直接删除。

商品图片支持上传 PNG、JPG、GIF，单张最大 2 MB，尺寸不超过 4096 × 4096。支付 Logo 和站点图片的具体设置见对应配置文档。

## 支付渠道

| 渠道 | 当前实现 | 配置说明 |
| --- | --- | --- |
| Stripe | Checkout 一次性付款，人民币订单，支持银行卡、支付宝和微信支付 | 实际可用方式取决于 Stripe 账户地区、开通情况及交易条件，需要配置 Webhook |
| 易支付 / ZPAY | `submit.php` POST / GET 跳转、`mapi.php` API 下单、MD5 通知、主动查单及未发货订单原路全额退款 | 支持渠道 ID、收银台链接与二维码；当前一个配置对应一种支付类型 |
| DujiaoPay | HMAC API 下单与独立 Webhook 验签 | 默认对外显示为 USDT 支付，需在商户侧配置到账通知及实际支持的网络 |
| BEpusdt | `create-transaction` 下单与 JSON MD5 回调 | 需要独立部署 BEpusdt 网关，商城部署脚本不安装该网关 |

顾客看到的是后台设置的支付方式名称与 Logo；商户密钥仅在后台配置并加密保存。名称只影响展示，不改变真实支付类型、币种或网络。

商品价格以人民币计价，USDT 的实际支付数量以网关收银台的汇率与金额为准。Stripe 当前用于一次性订单付款，不提供会员自动续费订阅。

商户申请、密钥获取、回调地址、支付宝及微信开通、测试与退款说明见 [支付渠道配置指南](docs/支付渠道配置指南.md)。

## 本地开发

准备 Go 1.25 或兼容版本、Node.js 22 LTS 与 npm。

### 启动后端

```bash
cd backend
go mod download
go run ./cmd/server
```

后端默认监听 `http://127.0.0.1:8080`，读取 `backend/configs/config.yaml`，使用 SQLite，并在首次启动时建表、创建管理员与停用状态的支付渠道。

### 启动前端

在另一个终端执行：

```bash
cd frontend
npm ci
npm run dev
```

默认访问 `http://127.0.0.1:5173`，端口占用时以 Vite 输出为准。开发服务器将 `/api` 和 `/uploads` 代理到后端。

本地初始后台入口为 `/admin`，用户名 `admin`，密码 `admin123`。这些是开发默认值；原生生产部署使用随机密码。管理员环境变量仅用于首次创建账号，已有账号应在后台改密。

只查看前端演示数据时，可以显式开启演示模式：

```bash
cd frontend
VITE_DEMO_MODE=true npm run dev
```

演示模式不进行真实下单、收款或发卡。正常模式使用后端数据，接口失败时不会自动切换到演示数据。

### 配置项

后端支持使用环境变量覆盖 YAML 配置：

| 变量 | 用途 |
| --- | --- |
| `SERVER_ADDR` | 后端监听地址 |
| `SERVER_MODE` | 运行模式 |
| `SERVER_PUBLIC_HOST` | 主站域名；启用分站后用于区分主站与已登记的分站 |
| `SERVER_STATION_ROOT` | 分站根域名，需同时配置泛域名 DNS 与 HTTPS |
| `DB_DRIVER` | 数据库类型，`sqlite` 或 `postgres` |
| `DB_DSN` | SQLite 文件路径或 PostgreSQL 连接串 |
| `JWT_SECRET` | 登录签名及敏感配置加密密钥，生产环境使用稳定的随机密钥 |
| `ADMIN_USERNAME` | 首次创建的管理员用户名 |
| `ADMIN_PASSWORD` | 首次创建的管理员密码 |
| `UPLOAD_DIR` | 上传文件存储目录 |

后端不会自动加载 `.env` 文件；本地需自行将变量导入进程环境。原生部署由脚本生成并管理运行配置。

前端配置参考 [frontend/.env.example](frontend/.env.example)，包括 `VITE_API_BASE_URL` 和 `VITE_DEMO_MODE`。正式构建应关闭演示模式。

### 检查与构建

```bash
# 在 backend 目录执行
go test ./...
go vet ./...

# 在 frontend 目录执行
npm ci
npm run build
```

前端构建包含 TypeScript 检查，产物位于 `frontend/dist`。生产部署优先使用根目录脚本，避免遗漏数据库、反向代理或上传文件配置。

## 项目结构

| 路径 | 内容 |
| --- | --- |
| `frontend/src/` | 顾客商城、订单页面与共享逻辑 |
| `frontend/src/admin/` | 后台管理页面 |
| `frontend/public/` | 前端静态资源 |
| `backend/cmd/` | 服务入口、管理员恢复、数据库恢复等工具 |
| `backend/internal/` | 配置、模型、业务服务、HTTP 接口、中间件、路由与支付实现 |
| `backend/configs/` | 后端配置模板 |
| `deploy.sh` | 原生部署与更新入口 |
| `deploy/` | 部署模板、备份脚本及部署验证 |
| `docs/` | 部署、配置、开发与运营文档 |
| `design-system/` | 界面设计规范 |
| `assets/` | 品牌资源 |

## 使用说明

顾客通过订单号和下单时填写的联系方式查询订单，领取交付内容并查看兑换入口。手工发货订单在管理员完成交付后展示结果。

「供应商与采购」用于资料、采购记录和卡密入库；「站点互联」用于独立 Wybo 之间的商品同步、自动采购及下游供货，采购钱包与代理佣金分别记账。第一版支持人民币自动卡密 / 账号整单交付，上游结果不确定时先查原单，售后按采购链路核查。配置见 [站点互联指南](docs/站点互联.md)。代理结算记录用于业务登记，不会自动向代理转账。退款是否支持直接调用支付接口，以渠道、订单状态及后台可用操作为准。

项目与 OpenAI、Anthropic、xAI 等品牌无隶属关系。

## 文档

- [原生部署与更新](docs/原生部署.md)
- [代理分站](docs/代理分站.md)
- [站点互联](docs/站点互联.md)
- [后台管理](docs/后台管理.md)
- [支付渠道配置指南](docs/支付渠道配置指南.md)
- [站点设置与人机验证](docs/站点设置与人机验证.md)
- [经营功能与数据恢复](docs/经营功能与数据恢复.md)
- [上线与经营扩展](docs/上线与经营扩展.md)
- [开发文档](docs/开发文档.md)

## 许可证

MIT。第三方资源说明及许可见 [品牌资源说明](frontend/public/brands/README.md) 和 [支付资源说明](frontend/public/payments/README.md)。
