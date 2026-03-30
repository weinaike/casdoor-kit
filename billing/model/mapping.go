package model

import "time"

// ProductEntitlementMapping maps a Casdoor product to local entitlement configuration.
type ProductEntitlementMapping struct {
	ID                 int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	CasdoorProductName string          `gorm:"type:varchar(128);uniqueIndex;not null" json:"casdoor_product_name"`
	CasdoorOwner       string          `gorm:"type:varchar(100);not null;default:'admin'" json:"casdoor_owner"`
	QuotaSeconds       int64           `gorm:"not null" json:"quota_seconds"`
	EntitlementType    EntitlementType `gorm:"type:varchar(20);not null" json:"entitlement_type"`
	PeriodMonths       int             `gorm:"default:0" json:"period_months"`
	PeriodDays         int             `gorm:"default:0" json:"period_days"`        // Alternative to PeriodMonths for shorter trials
	MaxPerUser         int             `gorm:"default:0" json:"max_per_user"`        // Max purchases per user (0 = unlimited)
	Description        string          `gorm:"type:text" json:"description"`
	IsActive           bool            `gorm:"not null;default:true" json:"is_active"`
	CreatedAt          time.Time       `gorm:"autoCreateTime" json:"created_at"`
}

func (ProductEntitlementMapping) TableName() string { return "product_entitlement_mapping" }
