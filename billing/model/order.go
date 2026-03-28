package model

import "time"

// OrderStatus represents the status of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "Pending"
	OrderStatusPaid      OrderStatus = "Paid"
	OrderStatusCancelled OrderStatus = "Cancelled"
)

// UserOrder is a local order record synced from Casdoor.
type UserOrder struct {
	ID                 int64       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID             string      `gorm:"type:varchar(64);index;not null" json:"user_id"`
	CasdoorOrderName   string      `gorm:"type:varchar(128);uniqueIndex;not null" json:"casdoor_order_name"`
	CasdoorProductName string      `gorm:"type:varchar(128);not null" json:"casdoor_product_name"`
	Price              float64     `gorm:"type:decimal(10,2);not null" json:"price"`
	Currency           string      `gorm:"type:varchar(10);not null;default:'CNY'" json:"currency"`
	Status             OrderStatus `gorm:"type:varchar(20);not null" json:"status"`
	GrantedSeconds     *int64      `json:"granted_seconds,omitempty"`
	EntitlementID      *int64      `json:"entitlement_id,omitempty"`
	CreatedAt          time.Time   `gorm:"autoCreateTime" json:"created_at"`
	PaidAt             *time.Time  `json:"paid_at,omitempty"`
}

func (UserOrder) TableName() string { return "user_order" }
