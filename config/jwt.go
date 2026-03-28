package config

// JWTConfig holds JWT authentication configuration.
type JWTConfig struct {
	PublicKeyPath  string
	PrivateKeyPath string
	Issuer         string
	AccessTokenTTL int // seconds
}
