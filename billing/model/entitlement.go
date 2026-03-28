package model

import "time"

// EntitlementType represents the type of entitlement.
type EntitlementType string

const (
	EntitlementTypeSubscription EntitlementType = "SUBSCRIPTION" // recurring subscription
	EntitlementTypeTopUp        EntitlementType = "TOP_UP"       // one-time top-up (permanent)
	EntitlementTypeGift         EntitlementType = "GIFT"         // gifted credits
)

// EntitlementStatus represents the status of an entitlement.
type EntitlementStatus string

const (
	EntitlementStatusActive    EntitlementStatus = "ACTIVE"    // active
	EntitlementStatusExhausted EntitlementStatus = "EXHAUSTED" // fully consumed
	EntitlementStatusExpired   EntitlementStatus = "EXPIRED"   // expired
)

// UserEntitlement represents a user's entitlement package.
type UserEntitlement struct {
	ID                 int64             `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID             string            `gorm:"type:varchar(64);index;not null" json:"user_id"`
	SourceType         EntitlementType   `gorm:"type:varchar(20);not null" json:"source_type"`
	TotalSeconds       int64             `gorm:"not null" json:"total_seconds"`
	UsedSeconds        int64             `gorm:"not null;default:0" json:"used_seconds"`
	FrozenSeconds      int64             `gorm:"not null;default:0" json:"frozen_seconds"`
	ValidFrom          time.Time         `gorm:"not null" json:"valid_from"`
	ValidUntil         *time.Time        `json:"valid_until,omitempty"`
	Status             EntitlementStatus `gorm:"type:varchar(20);not null" json:"status"`
	CasdoorProductName string            `gorm:"type:varchar(128)" json:"casdoor_product_name"`
	OrderID            *int64            `json:"order_id,omitempty"`
	CreatedAt          time.Time         `gorm:"autoCreateTime" json:"created_at"`
}

func (UserEntitlement) TableName() string { return "user_entitlement" }

// AvailableSeconds returns the remaining available seconds.
func (e *UserEntitlement) AvailableSeconds() int64 {
	return e.TotalSeconds - e.UsedSeconds - e.FrozenSeconds
}
