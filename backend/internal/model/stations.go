package model

import "time"

// Station is a hosted reseller shop. AgentID retains the existing settlement ledger.
type Station struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	AgentID         uint      `gorm:"uniqueIndex;not null" json:"agent_id"`
	Slug            string    `gorm:"size:32;uniqueIndex;not null" json:"slug"`
	Status          string    `gorm:"size:16;not null;default:active" json:"status"`
	SiteName        string    `gorm:"size:180" json:"site_name"`
	SiteBrand       string    `gorm:"size:180" json:"site_brand"`
	Logo            string    `json:"site_logo"`
	Favicon         string    `json:"site_favicon"`
	Description     string    `json:"site_desc"`
	Announcement    string    `json:"announcement"`
	CustomerService string    `json:"customer_service"`
	RedeemURL       string    `json:"default_redeem_url"`
	RedeemGuide     string    `json:"redeem_guide"`
	Email           string    `gorm:"size:254" json:"email"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type StationProduct struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	StationID      uint      `gorm:"uniqueIndex:station_product;not null" json:"station_id"`
	ProductID      uint      `gorm:"uniqueIndex:station_product;not null" json:"product_id"`
	WholesaleCents int64     `json:"wholesale_cents"`
	RetailCents    int64     `json:"retail_cents"`
	Authorized     bool      `json:"authorized"`
	Listed         bool      `json:"listed"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type StationApplication struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`
	Slug        string    `gorm:"index;size:32" json:"slug"`
	Email       string    `gorm:"index;size:254" json:"email"`
	Contact     string    `json:"contact"`
	Description string    `json:"description"`
	Status      string    `gorm:"index;size:16;default:pending" json:"status"`
	ReviewNote  string    `json:"review_note"`
	StationID   *uint     `json:"station_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type StationInvitation struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	StationID      uint       `gorm:"index" json:"station_id"`
	TokenHash      string     `gorm:"uniqueIndex;size:64" json:"-"`
	TokenEncrypted string     `json:"-"`
	ExpiresAt      time.Time  `json:"expires_at"`
	UsedAt         *time.Time `json:"used_at"`
	Status         string     `gorm:"index" json:"status"`
	Attempts       int        `json:"attempts"`
	LastError      string     `json:"last_error"`
	NextAttemptAt  time.Time  `json:"next_attempt_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type StationAudit struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	StationID uint      `gorm:"index" json:"station_id"`
	Action    string    `json:"action"`
	CreatedAt time.Time `json:"created_at"`
}

type StationUpload struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	StationID uint      `gorm:"index" json:"station_id"`
	URL       string    `gorm:"uniqueIndex" json:"url"`
	CreatedAt time.Time `json:"created_at"`
}
