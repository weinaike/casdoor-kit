package model

import "time"

// BillingActionType represents the type of billing action.
type BillingActionType string

const (
	BillingActionFreeze   BillingActionType = "FREEZE"
	BillingActionConsume  BillingActionType = "CONSUME"
	BillingActionUnfreeze BillingActionType = "UNFREEZE"
	BillingActionGrant    BillingActionType = "GRANT"
	BillingActionExpire   BillingActionType = "EXPIRE"
)

// BillingTransactionLog is an immutable audit trail of billing actions.
type BillingTransactionLog struct {
	ID            int64             `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        string            `gorm:"type:varchar(64);index;not null" json:"user_id"`
	TaskRef       *string           `gorm:"type:uuid;column:job_uuid" json:"job_uuid,omitempty"`
	EntitlementID *int64            `json:"entitlement_id,omitempty"`
	ActionType    BillingActionType `gorm:"type:varchar(20);not null" json:"action_type"`
	AmountSeconds int64             `gorm:"not null" json:"amount_seconds"`
	BalanceAfter  int64             `gorm:"not null" json:"balance_after"`
	Description   string            `gorm:"type:text" json:"description"`
	CreatedAt     time.Time         `gorm:"autoCreateTime" json:"created_at"`
}

func (BillingTransactionLog) TableName() string { return "billing_transaction_log" }
