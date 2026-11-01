# Wybo · 望月AI代充小铺

> AI 会员自动发卡网（卡密/CDK 售卖平台）。用户在线支付成功后，系统自动发放一条卡密（CDK），凭卡密到指定官网兑换即可。

- 品牌：**Wybo** / 系统名：**望月AI代充小铺**
- 主营：ChatGPT / Claude / Grok 等 AI 会员充值
- 视觉：可爱卡通「望月」主题（夜空 + 满月 + 小兔子）

---

## ✨ 功能特性

- **自动发卡**：支付回调验签 → 幂等处理 → 事务发卡（防超卖）→ 自动扣减库存
- **卡密管理**：批量导入（支持「单码」与「账号,密码」双格式）、自动去重、停用/删除
- **多支付网关**：Stripe、易支付、DujiaoPay、BEpusdt 服务端下单与签名回调
- **运营能力**：优惠券（满减/折扣）、公告、站点设置、人工补卡、分销代理、Resend 邮件和成交推送
- **定时任务**：订单超时自动关闭
- **完整后台**：经营利润与流量仪表盘、商品成本/售价、卡密、订单、代理、支付、邮件推送、安全与 2FA
- **可爱前台**：分类、商品卡片、下单、支付、卡密查询

## 🧱 技术栈

| 层 | 技术 |
|----|------|
| 前端 | React 18 + TypeScript + Vite + React Router + Recharts + Lucide |
| 后端 | Golang + Gin + GORM v2 + Viper + zap + JWT |
| 数据库 | SQLite（本地开发）/ PostgreSQL（正式环境，GORM 免改代码切换） |
| 支付 | 自研 Gateway 抽象（Stripe / 易支付 / DujiaoPay / BEpusdt） |

## 📁 目录结构

```
Wybo/
├── backend/          # Golang 后端
│   ├── cmd/server/   # 入口 main.go
│   ├── internal/
│   │   ├── config/   # 配置加载
│   │   ├── model/    # GORM 模型 + 自动迁移
│   │   ├── service/  # 业务层（订单/发卡/卡密/优惠券/公告/设置）
│   │   ├── handler/  # HTTP 处理层
│   │   ├── middleware/# JWT / CORS
│   │   ├── router/   # 路由
│   │   ├── pay/      # 支付网关抽象与实现
│   │   └── cron/     # 定时任务（订单超时关闭）
│   ├── pkg/response/ # 统一响应
│   └── configs/      # 配置模板
├── frontend/         # React 前端
│   └── src/
│       ├── pages/    # 前台页面
│       ├── admin/    # 后台管理页面
│       ├── components/ # 通用组件（Logo / ProductCard）
│       ├── api/      # 接口封装
│       └── store/    # 状态
├── assets/           # 品牌资源（Logo / favicon）
├── docs/开发文档.md   # 完整开发文档
└── README.md
```

## 🚀 快速开始

### 后端（端口 8080）

```bash
cd backend
go mod tidy
go run ./cmd/server
```

首次启动会自动建表，并播种默认管理员与支付渠道（默认停用）。

### 前端（端口 5173，/api 自动代理到 8080）

```bash
cd frontend
npm install
npm run dev
```

浏览器打开 <http://localhost:5173>。

### 默认账号

| 项 | 值 |
|----|----|
| 后台地址 | http://localhost:5173/admin |
| 用户名 | `admin` |
| 密码 | `admin123` |

> 首次初始化可用环境变量设置账号；已有账号在后台安全页面改密。正式部署设置稳定的 JWT_SECRET，详见后台管理文档。

## ⚙️ 环境变量

后端通过环境变量覆盖 `configs/config.yaml`：

| 变量 | 默认 | 说明 |
|------|------|------|
| `SERVER_ADDR` | `:8080` | 监听地址 |
| `DB_DRIVER` | `sqlite` | `sqlite`（本地）/ `postgres`（正式） |
| `DB_DSN` | `data/wybo.db` | SQLite 文件路径 / PostgreSQL 连接串 |
| `JWT_SECRET` | `change-me` | JWT 签名密钥 |
| `ADMIN_USERNAME` | `admin` | 默认管理员用户名 |
| `ADMIN_PASSWORD` | `admin123` | 默认管理员密码 |

## 💳 支付网关接入

网关密钥在**后台「支付渠道」**配置，加密保存，API 返回遮罩。启用前需填完整商户信息和公网回调地址。

- **易支付（已实现）**：填写 `api_url`、`pid`、`key`、`type`（`alipay`/`wxpay`），启用后即可在商品支付页发起。
- **Stripe**：CNY Checkout、银行卡 / 支付宝 / 微信选择与 Webhook 验签，实际可用方式取决于 Stripe 账户开通情况。
- **DujiaoPay**：HMAC API、独立 Webhook 密钥、幂等下单。
- **BEpusdt**：create-transaction 下单、JSON MD5 回调。

配置步骤、利润口径、Resend、代理、2FA 和本地恢复工具见 [docs/后台管理.md](docs/后台管理.md)。商户密钥和邮件域名仍需自行配置，未进行真实收款或邮件发送。

## 🔄 核心业务流程

```
用户选购 → 提交订单(pending) → 选择支付方式 → 网关下单
    → 用户支付 → 网关异步回调 → 后端验签 + 幂等
    → 订单 pending→paid → 事务发卡 → completed → 用户查看/找回卡密
```

## 📡 API 概览

统一前缀 `/api/v1`，返回 `{ code, message, data }`，`code=0` 表示成功。

**公开接口**：`GET /site`、`/categories`、`/products`、`/products/:id`、`/announcements`、`/orders/:no`、`/query`；`POST /orders`、`/orders/:no/pay`、`/pay/notify/:gateway`。

**管理接口**（`/admin/*`，JWT）：登录、仪表盘、商品/分类/卡密/订单/优惠券/公告/支付/设置 的完整增删改查，以及订单退款、人工补卡、卡密停用删除。

详细接口见 [docs/开发文档.md](docs/开发文档.md)。

## 📦 原生部署（不使用 Docker）

完整部署脚本会安装 PostgreSQL、Nginx、独立 Go / Node 工具链，生成随机数据库 / 管理员密码，配置 SSL 自动续期、systemd 与每日备份。支持 Ubuntu 22.04 / 24.04、Debian 12 / 13。2FA 由用户登录后台后自行开启。

```bash
sudo bash deploy.sh install --domain shop.example.com --email ops@example.com
```

先上传完整项目并把域名解析到服务器。操作、升级、凭据获取和恢复说明见 [原生部署指南](docs/原生部署.md)，收款配置见 [支付渠道配置指南](docs/支付渠道配置指南.md)。首次部署是新的 PostgreSQL 数据库，不会自动迁移本地 SQLite 数据。

只需手工构建时：

```bash
# 后端：编译为单二进制
cd backend && CGO_ENABLED=0 go build -o bin/wybo-server ./cmd/server

# 前端：构建静态产物
cd frontend && npm run build   # 产出 frontend/dist

# 用 Nginx 托管 dist 并反代 /api 到 8080（见开发文档 §15）
```

SQLite 使用 backup API 或停服复制；PostgreSQL 用 `pg_dump`。

## 🔒 安全提示

- 生产环境务必修改默认管理员密码与 `JWT_SECRET`，并启用 HTTPS。
- 支付回调必须依赖网关验签；卡密仅在支付成功后发放，查询接口校验「订单号 + 联系方式」双匹配。
- 本系统为卡密自动发卡平台，与 OpenAI / Anthropic / xAI 无隶属关系。

## 📄 License

MIT（开源参考：独角数卡 / 异次元发卡 / gptibo）。

经营功能、支付对账、代理结算、营销、售后与数据恢复说明：[docs/经营功能与数据恢复.md](docs/经营功能与数据恢复.md)。
