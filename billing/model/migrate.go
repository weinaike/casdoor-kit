package model

import "gorm.io/gorm"

// AutoMigrate runs database migrations for all billing tables.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&UserWallet{},
		&UserEntitlement{},
		&UserOrder{},
		&ProductEntitlementMapping{},
		&BillingTransactionLog{},
		&TaskBilling{},
	)
}
