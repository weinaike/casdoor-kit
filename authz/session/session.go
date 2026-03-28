package session

import "context"

// CasdoorSession stores Casdoor tokens for a user.
type CasdoorSession struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresAt        int64  `json:"expires_at"`
	RefreshExpiresAt int64  `json:"refresh_expires_at"`
}

// SessionService manages user sessions.
type SessionService interface {
	StoreSession(ctx context.Context, userID string, session *CasdoorSession) error
	GetSession(ctx context.Context, userID string) (*CasdoorSession, error)
	DeleteSession(ctx context.Context, userID string) error
}
