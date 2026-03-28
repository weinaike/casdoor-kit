package config

// CasdoorConfig holds Casdoor OAuth2 configuration.
type CasdoorConfig struct {
	Endpoint     string
	ClientID     string
	ClientSecret string
	Organization string
	Application  string
	RedirectURI  string
}
