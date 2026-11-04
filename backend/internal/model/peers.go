package model

import "time"

type PeerAccount struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:80;not null" json:"name"`
	APIKey       string    `gorm:"size:64;uniqueIndex;not null" json:"api_key"`
	Secret       string    `json:"-"`
	CallbackURL  string    `gorm:"size:500" json:"callback_url"`
	Enabled      bool      `json:"enabled"`
	BalanceCents int64     `gorm:"not null;default:0" json:"balance_cents"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type PeerGrant struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AccountID  uint      `gorm:"uniqueIndex:peer_grant" json:"account_id"`
	ProductID  uint      `gorm:"uniqueIndex:peer_grant" json:"product_id"`
	PriceCents int64     `json:"price_cents"`
	Enabled    bool      `json:"enabled"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type PeerLedger struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	AccountID    uint      `gorm:"index;uniqueIndex:peer_ledger_ref" json:"account_id"`
	Kind         string    `gorm:"size:16;uniqueIndex:peer_ledger_ref" json:"kind"`
	Reference    string    `gorm:"size:160;uniqueIndex:peer_ledger_ref" json:"reference"`
	AmountCents  int64     `json:"amount_cents"`
	BalanceCents int64     `json:"balance_cents"`
	Note         string    `gorm:"size:500" json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}
type PeerQuote struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	AccountID  uint      `gorm:"index" json:"-"`
	ProductID  uint      `json:"product_id"`
	Quantity   int       `json:"quantity"`
	TotalCents int64     `json:"total_cents"`
	Route      string    `json:"-"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
}
type PeerInbound struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	AccountID   uint      `gorm:"uniqueIndex:peer_external" json:"account_id"`
	ExternalNo  string    `gorm:"size:120;uniqueIndex:peer_external" json:"external_no"`
	OrderID     uint      `gorm:"uniqueIndex" json:"order_id"`
	RequestHash string    `gorm:"size:64" json:"-"`
	Route       string    `json:"-"`
	Version     uint      `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
}
type PeerNonce struct {
	ID        uint      `gorm:"primaryKey"`
	Scope     string    `gorm:"size:120;uniqueIndex:peer_nonce"`
	Nonce     string    `gorm:"size:80;uniqueIndex:peer_nonce"`
	CreatedAt time.Time `gorm:"index"`
}
type PeerConnection struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Name            string     `gorm:"size:80" json:"name"`
	BaseURL         string     `gorm:"size:500" json:"base_url"`
	APIKey          string     `gorm:"size:64" json:"api_key"`
	Secret          string     `json:"-"`
	Enabled         bool       `json:"enabled"`
	SiteID          string     `gorm:"size:64" json:"site_id"`
	RemoteAccountID uint       `json:"-"`
	BalanceCents    int64      `json:"balance_cents"`
	LastError       string     `gorm:"size:500" json:"last_error"`
	LastSyncAt      *time.Time `json:"last_sync_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
type PeerCatalog struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ConnectionID    uint      `gorm:"uniqueIndex:peer_catalog" json:"connection_id"`
	RemoteProductID uint      `gorm:"uniqueIndex:peer_catalog" json:"remote_product_id"`
	Name            string    `gorm:"size:128" json:"name"`
	Description     string    `json:"description"`
	PriceCents      int64     `json:"price_cents"`
	Stock           int       `json:"stock"`
	DeliveryMode    string    `json:"delivery_mode"`
	Route           string    `json:"route"`
	Enabled         bool      `json:"enabled"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type PeerMapping struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ProductID        uint      `gorm:"uniqueIndex" json:"product_id"`
	ConnectionID     uint      `json:"connection_id"`
	RemoteProductID  uint      `json:"remote_product_id"`
	MaxUnitCostCents int64     `json:"max_unit_cost_cents"`
	Enabled          bool      `json:"enabled"`
	SyncPrice        bool      `json:"sync_price"`
	MarkupBps        int64     `json:"markup_bps"`
	MarkupCents      int64     `json:"markup_cents"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type PeerJob struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	OrderID         uint       `gorm:"uniqueIndex" json:"order_id"`
	OrderNo         string     `gorm:"size:32" json:"order_no"`
	ConnectionID    uint       `json:"connection_id"`
	RemoteProductID uint       `json:"-"`
	Quantity        int        `json:"-"`
	MaxTotalCents   int64      `json:"-"`
	QuoteID         string     `json:"-"`
	QuoteTotalCents int64      `json:"-"`
	RemoteVersion   uint       `json:"-"`
	ExternalNo      string     `gorm:"size:120;uniqueIndex" json:"-"`
	RemoteOrderNo   string     `json:"remote_order_no"`
	Route           string     `json:"-"`
	Status          string     `gorm:"index" json:"status"`
	Attempts        int        `json:"attempts"`
	NextAttemptAt   time.Time  `gorm:"index" json:"next_attempt_at"`
	LockedAt        *time.Time `json:"-"`
	LastError       string     `gorm:"size:500" json:"last_error"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
type PeerEvent struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	EventID       string     `gorm:"size:64;uniqueIndex" json:"event_id"`
	OrderID       uint       `gorm:"uniqueIndex:peer_event_version" json:"order_id"`
	Version       uint       `gorm:"uniqueIndex:peer_event_version" json:"version"`
	AccountID     uint       `json:"account_id"`
	Status        string     `gorm:"index" json:"status"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt time.Time  `gorm:"index" json:"next_attempt_at"`
	LockedAt      *time.Time `json:"-"`
	LastError     string     `gorm:"size:500" json:"last_error"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
