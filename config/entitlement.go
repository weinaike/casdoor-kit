package config

// EntitlementConfig holds entitlement (billing quota) configuration.
type EntitlementConfig struct {
	ProductMappings map[string]ProductMapping
}

// ProductMapping maps a Casdoor product to an entitlement quota.
type ProductMapping struct {
	QuotaSeconds    int64  // quota duration in seconds
	EntitlementType string // TOP_UP / SUBSCRIPTION / GIFT / TRIAL
	PeriodMonths    int    // validity period in months (0 = permanent)
	PeriodDays      int    // validity period in days (alternative to PeriodMonths, for shorter periods)
	MaxPerUser      int    // maximum times a user can purchase this product (0 = unlimited)
	Description     string // display description
}
