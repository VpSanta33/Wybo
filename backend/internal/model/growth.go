package model

import "time"

type Supplier struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `json:"name"`
	Contact   string    `json:"contact"`
	Note      string    `json:"note"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Purchase struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Number        string     `gorm:"uniqueIndex" json:"number"`
	SupplierID    uint       `gorm:"index" json:"supplier_id"`
	ProductID     uint       `gorm:"index" json:"product_id"`
	Quantity      int        `json:"quantity"`
	UnitCostCents int64      `json:"unit_cost_cents"`
	PaidCents     int64      `json:"paid_cents"`
	Status        string     `json:"status"`
	BatchNo       string     `json:"batch_no"`
	Note          string     `json:"note"`
	ReceivedAt    *time.Time `json:"received_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
type PurchasePayment struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	PurchaseID  uint      `gorm:"index" json:"purchase_id"`
	AmountCents int64     `json:"amount_cents"`
	Reference   string    `gorm:"uniqueIndex" json:"reference"`
	CreatedAt   time.Time `json:"created_at"`
}
type AfterSale struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Number         string    `gorm:"uniqueIndex" json:"number"`
	OrderID        uint      `gorm:"index" json:"-"`
	OrderNo        string    `json:"order_no"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	OriginalCardID *uint     `gorm:"uniqueIndex" json:"original_card_id"`
	NewCardID      *uint     `json:"new_card_id"`
	ChildOrderID   *uint     `json:"-"`
	ChildOrderNo   string    `json:"child_order_no"`
	AmountCents    int64     `json:"amount_cents"`
	Reason         string    `json:"reason"`
	Reference      string    `json:"reference"`
	CreatedAt      time.Time `json:"created_at"`
}
type HealthAlert struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"uniqueIndex" json:"key"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail"`
	Status    string    `json:"status"`
	Cycle     string    `json:"-"`
	Notified  bool      `json:"notified"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Diagnostic struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Kind      string    `gorm:"index" json:"kind"`
	Channel   string    `json:"channel"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}
type FunnelEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Day       string    `gorm:"index;uniqueIndex:funnel_event" json:"day"`
	Visitor   string    `gorm:"uniqueIndex:funnel_event" json:"-"`
	Kind      string    `gorm:"uniqueIndex:funnel_event" json:"kind"`
	ProductID uint      `gorm:"uniqueIndex:funnel_event" json:"product_id"`
	Source    string    `json:"source"`
	Campaign  string    `json:"campaign"`
	CreatedAt time.Time `json:"created_at"`
}
