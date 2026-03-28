package model

import "time"

// TaskBillingStatus represents the billing status of a task.
type TaskBillingStatus string

const (
	TaskBillingStatusPending    TaskBillingStatus = "PENDING"
	TaskBillingStatusProcessing TaskBillingStatus = "PROCESSING"
	TaskBillingStatusSuccess    TaskBillingStatus = "SUCCESS"
	TaskBillingStatusFailed     TaskBillingStatus = "FAILED"
)

// TaskBilling links a task to its billing record.
// The Go field is TaskRef but the DB column remains job_uuid for backward compatibility.
type TaskBilling struct {
	ID            int64             `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskRef       string            `gorm:"type:uuid;uniqueIndex;not null;column:job_uuid" json:"job_uuid"`
	UserID        string            `gorm:"type:varchar(64);index;not null" json:"user_id"`
	JobSeconds    int64             `gorm:"not null" json:"job_seconds"`
	BilledSeconds int64             `gorm:"not null" json:"billed_seconds"`
	Status        TaskBillingStatus `gorm:"type:varchar(20);not null" json:"status"`
	FrozenDetails FrozenDetails     `gorm:"type:jsonb;not null" json:"frozen_details"`
	CreatedAt     time.Time         `gorm:"autoCreateTime" json:"created_at"`
}

func (TaskBilling) TableName() string { return "job_billing" }
