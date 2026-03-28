package model

import "time"

// UserWallet tracks a user's balance summary.
type UserWallet struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"user_id"`
	TotalAvailable int64     `gorm:"not null;default:0" json:"total_available"`
	TotalFrozen    int64     `gorm:"not null;default:0" json:"total_frozen"`
	Version        int       `gorm:"not null;default:1" json:"version"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserWallet) TableName() string { return "user_wallet" }

// AvailableSeconds returns the available balance.
func (w *UserWallet) AvailableSeconds() int64 {
	return w.TotalAvailable - w.TotalFrozen
}
