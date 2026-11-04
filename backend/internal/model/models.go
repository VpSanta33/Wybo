package model

import "time"

// 本文件定义全部数据表模型与业务枚举常量。
// 金额统一以「分」(int64) 存储，避免浮点误差。

// ============ 管理员 ============

const (
	AdminRoleSuper    = "super"    // 超级管理员
	AdminRoleOperator = "operator" // 运营
)

type Admin struct {
	Disabled       bool       `gorm:"default:false" json:"disabled"`
	ID             uint       `gorm:"primaryKey" json:"id"`
	Username       string     `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash   string     `gorm:"size:255;not null" json:"-"`
	SessionVersion int64      `gorm:"default:0" json:"-"`
	TOTPSecret     string     `json:"-"`
	TOTPPending    string     `json:"-"`
	TOTPExpiresAt  *time.Time `json:"-"`
	LastTOTPStep   int64      `gorm:"default:0" json:"-"`
	Role           string     `gorm:"size:16;not null;default:operator" json:"role"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ============ 商品分类 ============

type Category struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Icon      string    `gorm:"size:255" json:"icon"`
	Sort      int       `gorm:"default:0" json:"sort"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ============ 商品 ============

const (
	DeliveryAuto   = "auto"
	DeliveryManual = "manual"
)

type Product struct {
	DeliveryMode       string    `gorm:"size:16;not null;default:auto" json:"delivery_mode"`
	ID                 uint      `gorm:"primaryKey" json:"id"`
	CategoryID         uint      `gorm:"index;not null" json:"category_id"`
	Name               string    `gorm:"size:128;not null" json:"name"`
	Description        string    `gorm:"type:text" json:"description"`
	Cover              string    `gorm:"size:255" json:"cover"`
	PriceCents         int64     `gorm:"not null" json:"price_cents"`
	CostCents          *int64    `json:"cost_cents"`
	OriginalPriceCents int64     `json:"original_price_cents"`
	RedeemURL          string    `gorm:"size:255" json:"redeem_url"`
	RedeemGuide        string    `gorm:"type:text" json:"redeem_guide"`
	Stock              int       `gorm:"default:0" json:"stock"` // 冗余：= available 卡密数
	Sales              int       `gorm:"default:0" json:"sales"`
	Enabled            bool      `gorm:"default:true" json:"enabled"`
	Sort               int       `gorm:"default:0" json:"sort"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ============ 卡密/CDK ============

const (
	CardAvailable = "available" // 可售
	CardSold      = "sold"      // 已售出
	CardDisabled  = "disabled"  // 已失效
)

type Card struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	ProductID uint       `gorm:"index:idx_product_status,priority:1;uniqueIndex:idx_product_code,priority:1;not null" json:"product_id"`
	Code      string     `gorm:"size:255;not null;uniqueIndex:idx_product_code,priority:2" json:"code"` // 卡密正文（单码/账号）
	Secret    string     `gorm:"size:255" json:"secret"`                                                // 卡密密码（账号+密码卡时使用）
	Status    string     `gorm:"size:16;index:idx_product_status,priority:2;default:available" json:"status"`
	OrderID   *uint      `gorm:"index" json:"order_id"` // 售出订单
	CostCents *int64     `json:"-"`
	BatchNo   string     `gorm:"size:64;index" json:"batch_no"` // 导入批次号
	SoldAt    *time.Time `json:"sold_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ============ 订单 ============

const (
	OrderPending   = "pending"   // 待支付
	OrderPaid      = "paid"      // 已支付待发卡
	OrderCompleted = "completed" // 已完成（已出卡）
	OrderClosed    = "closed"    // 已关闭（超时/取消）
	OrderRefunded  = "refunded"  // 已退款
)

type Order struct {
	StationID            *uint  `gorm:"index" json:"station_id"`
	WholesaleCents       *int64 `json:"wholesale_cents"`
	StationHost          string `json:"station_host"`
	StationFeePolicy     string `json:"station_fee_policy"`
	StationMarginCents   int64  `gorm:"default:0" json:"station_margin_cents"`
	CouponState          string `json:"-"`
	Visitor              string `gorm:"index" json:"-"`
	Source               string `json:"source"`
	Campaign             string `json:"campaign"`
	ParentOrderID        *uint  `gorm:"index" json:"parent_order_id"`
	ExtraCostCents       int64  `gorm:"default:0" json:"extra_cost_cents"`
	CommissionAdjustment int64  `gorm:"default:0" json:"commission_adjustment"`
	DeliveredRefund      bool   `gorm:"default:false" json:"delivered_refund"`

	ID              uint       `gorm:"primaryKey" json:"id"`
	OrderNo         string     `gorm:"size:32;uniqueIndex;not null" json:"order_no"`
	Status          string     `gorm:"size:16;index;default:pending" json:"status"`
	TotalCents      int64      `gorm:"not null" json:"total_cents"`
	DiscountCents   int64      `json:"discount_cents"`
	CostCents       *int64     `json:"cost_cents"`
	CostSource      string     `json:"cost_source"`
	FeeCents        *int64     `json:"fee_cents"`
	FeeSource       string     `json:"fee_source"`
	CommissionBps   int64      `gorm:"default:0" json:"commission_bps"`
	CommissionCents int64      `gorm:"default:0" json:"commission_cents"`
	SettlementID    *uint      `gorm:"index" json:"settlement_id"`
	RefundedCents   int64      `gorm:"default:0" json:"refunded_cents"`
	AgentID         *uint      `gorm:"index" json:"agent_id"`
	AgentName       string     `json:"agent_name"`
	Contact         string     `gorm:"size:128;not null" json:"contact"` // 联系方式（查卡凭证）
	Email           string     `gorm:"size:128" json:"email"`
	CouponID        *uint      `json:"coupon_id"`
	PayMethod       string     `gorm:"size:32" json:"pay_method"`
	PaidAt          *time.Time `json:"paid_at"`
	ExpiredAt       time.Time  `json:"expired_at"`     // 超时关闭时间
	CleanedAt       *time.Time `gorm:"index" json:"-"` // 后台清理时间，保留查单与迟到支付处理
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ============ 订单明细 ============

type OrderItem struct {
	DeliveryMode   string `gorm:"size:16;not null;default:auto" json:"delivery_mode"` // 下单时的发货方式快照
	ID             uint   `gorm:"primaryKey" json:"id"`
	OrderID        uint   `gorm:"index;not null" json:"order_id"`
	ProductID      uint   `gorm:"index;not null" json:"product_id"`
	ProductName    string `gorm:"size:128" json:"product_name"` // 下单时快照
	Quantity       int    `gorm:"not null" json:"quantity"`
	UnitPriceCents int64  `gorm:"not null" json:"unit_price_cents"`
	UnitCostCents  *int64 `json:"-"`
}

// ============ 支付记录 ============

const (
	PaymentCreated = "created"
	PaymentSuccess = "success"
	PaymentFailed  = "failed"
)

type Payment struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	OrderID     uint       `gorm:"index;not null" json:"order_id"`
	PayOrderNo  string     `gorm:"size:64;uniqueIndex;not null" json:"pay_order_no"` // 幂等键
	Gateway     string     `gorm:"size:32" json:"gateway"`
	AmountCents int64      `gorm:"not null" json:"amount_cents"`
	Status      string     `gorm:"size:16;default:created" json:"status"`
	Raw         string     `gorm:"type:text" json:"raw"` // 回调原始报文（审计）
	PaidAt      *time.Time `json:"paid_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ============ 支付网关（配置） ============

const (
	GatewayEpay    = "epay"
	GatewayStripe  = "stripe"
	GatewayAlipay  = "alipay"
	GatewayWechat  = "wechat"
	GatewayUsdt    = "usdt"
	GatewayDujiao  = "dujiaopay"
	GatewayBepusdt = "bepusdt"
)

type Gateway struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:32;not null" json:"name"`             // 展示名
	Code      string    `gorm:"size:32;uniqueIndex;not null" json:"code"` // epay/stripe/...
	Enabled   bool      `gorm:"default:false" json:"enabled"`
	Config    string    `gorm:"type:text" json:"config"` // JSON：各网关密钥
	Sort      int       `gorm:"default:0" json:"sort"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ============ 优惠券 ============

const (
	CouponFixed   = "fixed"   // 满减
	CouponPercent = "percent" // 折扣
)

type Coupon struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Code           string     `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Type           string     `gorm:"size:16;not null" json:"type"`
	Value          int        `gorm:"not null" json:"value"` // 面额(分) 或 折扣(如 90=9折)
	ProductIDs     []uint     `gorm:"serializer:json" json:"product_ids"`
	MinAmountCents int64      `json:"min_amount_cents"`
	StartAt        *time.Time `json:"start_at"`
	EndAt          *time.Time `json:"end_at"`
	Total          int        `gorm:"default:0" json:"total"`
	Used           int        `gorm:"default:0" json:"used"`
	Enabled        bool       `gorm:"default:true" json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ============ 公告 ============

type Announcement struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"size:128;not null" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Sort      int       `gorm:"default:0" json:"sort"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ============ 站点配置（KV） ============

type Setting struct {
	Key       string    `gorm:"size:64;primaryKey" json:"key"`
	Value     string    `gorm:"type:text" json:"value"` // JSON 或字符串
	UpdatedAt time.Time `json:"updated_at"`
}

// Visitor identifiers are daily HMAC hashes; raw IPs and emails are not retained.
type Visit struct {
	StationID *uint     `gorm:"index" json:"station_id"`
	ID        uint      `gorm:"primaryKey" json:"id"`
	Day       string    `gorm:"size:10;index" json:"day"`
	Visitor   string    `gorm:"size:64;index" json:"-"`
	Path      string    `gorm:"size:255" json:"path"`
	Source    string    `gorm:"size:255" json:"source"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

type Agent struct {
	TOTPSecret     string     `json:"-"`
	TOTPPending    string     `json:"-"`
	TOTPExpiresAt  *time.Time `json:"-"`
	LastTOTPStep   int64      `gorm:"default:0" json:"-"`
	CommissionBps  int64      `gorm:"default:0" json:"commission_bps"`
	SettlementDays int        `gorm:"default:7" json:"settlement_days"`
	PasswordHash   string     `json:"-"`
	SessionVersion int64      `gorm:"default:0" json:"-"`
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:64;not null" json:"name"`
	Code           string     `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Contact        string     `gorm:"size:254" json:"contact"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
type AgentPrice struct {
	ID         uint  `gorm:"primaryKey" json:"id"`
	AgentID    uint  `gorm:"uniqueIndex:agent_product" json:"agent_id"`
	ProductID  uint  `gorm:"uniqueIndex:agent_product" json:"product_id"`
	PriceCents int64 `json:"price_cents"`
}

// Unique delivery jobs keep repeated payment callbacks from sending duplicates.
type Notification struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	OrderID       uint      `gorm:"uniqueIndex:order_channel" json:"order_id"`
	Channel       string    `gorm:"uniqueIndex:order_channel" json:"channel"`
	Status        string    `gorm:"index" json:"status"`
	Attempts      int       `json:"attempts"`
	LastError     string    `json:"last_error"`
	NextAttemptAt time.Time `gorm:"index" json:"next_attempt_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Operations tables never expose card contents or payment credentials.
type InventoryBatch struct {
	PurchaseID    *uint     `gorm:"index" json:"purchase_id"`
	SupplierID    *uint     `gorm:"index" json:"supplier_id"`
	ID            uint      `gorm:"primaryKey" json:"id"`
	BatchNo       string    `gorm:"uniqueIndex" json:"batch_no"`
	ProductID     uint      `gorm:"index" json:"product_id"`
	UnitCostCents *int64    `json:"unit_cost_cents"`
	Quantity      int       `json:"quantity"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}
type CheckoutAttempt struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	OrderID       uint       `gorm:"uniqueIndex:checkout_order_gateway" json:"order_id"`
	OrderNo       string     `gorm:"index" json:"order_no"`
	Gateway       string     `gorm:"uniqueIndex:checkout_order_gateway" json:"gateway"`
	ProviderID    string     `json:"provider_id"`
	Status        string     `gorm:"index" json:"status"`
	Result        string     `json:"-"`
	FeeBps        *int64     `json:"-"`
	FeeFixed      *int64     `json:"-"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	Message       string     `json:"message"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
type PaymentIssue struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrderNo   string    `json:"order_no"`
	Gateway   string    `json:"gateway"`
	Reference string    `gorm:"uniqueIndex:issue_reference" json:"reference"`
	Kind      string    `gorm:"uniqueIndex:issue_reference" json:"kind"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Refund struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	OrderID     uint      `gorm:"uniqueIndex" json:"order_id"`
	OrderNo     string    `json:"order_no"`
	Gateway     string    `json:"gateway"`
	AmountCents int64     `json:"amount_cents"`
	Status      string    `json:"status"`
	Reference   string    `json:"reference"`
	Reason      string    `json:"reason"`
	Mode        string    `json:"mode"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Settlement struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	AgentID     uint       `gorm:"index" json:"agent_id"`
	AmountCents int64      `json:"amount_cents"`
	OrderCount  int        `json:"order_count"`
	Status      string     `json:"status"`
	Reference   string     `json:"reference"`
	PaidAt      *time.Time `json:"paid_at"`
	CreatedAt   time.Time  `json:"created_at"`
}
type StockAlert struct {
	Snapshot    string    `json:"-"`
	Cycle       string    `json:"-"`
	ID          uint      `gorm:"primaryKey" json:"id"`
	ProductID   uint      `gorm:"uniqueIndex" json:"product_id"`
	ProductName string    `json:"product_name"`
	Stock       int       `json:"stock"`
	Threshold   int       `json:"threshold"`
	Status      string    `json:"status"`
	Notified    bool      `json:"notified"`
	Attempts    int       `json:"attempts"`
	LastError   string    `json:"last_error"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ActorID   uint      `json:"actor_id"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Result    int       `json:"result"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
type Backup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Filename  string    `json:"filename"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}
type Ticket struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Number    string    `gorm:"uniqueIndex" json:"number"`
	OrderID   uint      `gorm:"index" json:"-"`
	OrderNo   string    `json:"order_no"`
	Subject   string    `json:"subject"`
	Status    string    `gorm:"index" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type TicketMessage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TicketID  uint      `gorm:"index" json:"ticket_id"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	Internal  bool      `json:"internal"`
	CreatedAt time.Time `json:"created_at"`
}
