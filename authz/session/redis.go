package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// KVStore is a minimal key-value store interface for session storage.
type KVStore interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, keys ...string) error
}

// redisStore implements SessionService backed by Redis.
type redisStore struct {
	client    *redis.Client
	defaultTTL time.Duration
}

// NewRedisStore creates a session service backed by go-redis.
func NewRedisStore(client *redis.Client) SessionService {
	return &redisStore{
		client:     client,
		defaultTTL: 7 * 24 * time.Hour,
	}
}

func (s *redisStore) sessionKey(userID string) string {
	return fmt.Sprintf("session:casdoor:%s", userID)
}

func (s *redisStore) StoreSession(ctx context.Context, userID string, session *CasdoorSession) error {
	key := s.sessionKey(userID)

	ttl := s.defaultTTL
	if session.RefreshExpiresAt > 0 {
		ttl = time.Until(time.Unix(session.RefreshExpiresAt, 0))
		if ttl <= 0 {
			ttl = s.defaultTTL
		}
	}

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}

	return s.client.Set(ctx, key, data, ttl).Err()
}

func (s *redisStore) GetSession(ctx context.Context, userID string) (*CasdoorSession, error) {
	key := s.sessionKey(userID)
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var session CasdoorSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("反序列化会话失败: %w", err)
	}
	return &session, nil
}

func (s *redisStore) DeleteSession(ctx context.Context, userID string) error {
	key := s.sessionKey(userID)
	return s.client.Del(ctx, key).Err()
}
