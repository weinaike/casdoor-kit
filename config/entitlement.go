package config

// EntitlementConfig holds entitlement (billing quota) configuration.
type EntitlementConfig struct {
	ProductMappings map[string]ProductMapping
}

// ProductMapping maps a Casdoor product to an entitlement quota.
type ProductMapping struct {
	QuotaSeconds    int64  // quota duration in seconds
	EntitlementType string // TOP_UP / SUBSCRIPTION / GIFT
	PeriodMonths    int    // validity period (0 = permanent)
	Description     string // display description
}
