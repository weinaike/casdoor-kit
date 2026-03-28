//go:build integration

package session

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:32531"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	return client
}

func TestStoreAndGetSession(t *testing.T) {
	client := newTestRedisClient(t)
	defer client.Close()

	svc := NewRedisStore(client)
	ctx := context.Background()
	userID := "test-user-session-1"

	// Clean up
	client.Del(ctx, "session:casdoor:"+userID)

	session := &CasdoorSession{
		AccessToken:      "access-token-abc",
		RefreshToken:     "refresh-token-xyz",
		ExpiresAt:        time.Now().Add(time.Hour).Unix(),
		RefreshExpiresAt: time.Now().Add(7 * 24 * time.Hour).Unix(),
	}

	if err := svc.StoreSession(ctx, userID, session); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}

	got, err := svc.GetSession(ctx, userID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if got.AccessToken != "access-token-abc" {
		t.Errorf("access_token = %q, want %q", got.AccessToken, "access-token-abc")
	}
	if got.RefreshToken != "refresh-token-xyz" {
		t.Errorf("refresh_token = %q, want %q", got.RefreshToken, "refresh-token-xyz")
	}
	if got.ExpiresAt != session.ExpiresAt {
		t.Errorf("expires_at = %d, want %d", got.ExpiresAt, session.ExpiresAt)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	client := newTestRedisClient(t)
	defer client.Close()

	svc := NewRedisStore(client)
	ctx := context.Background()

	_, err := svc.GetSession(ctx, "nonexistent-user-12345")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestDeleteSession(t *testing.T) {
	client := newTestRedisClient(t)
	defer client.Close()

	svc := NewRedisStore(client)
	ctx := context.Background()
	userID := "test-user-session-del"

	client.Del(ctx, "session:casdoor:"+userID)

	session := &CasdoorSession{
		AccessToken:  "token-to-delete",
		RefreshToken: "refresh-to-delete",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}
	svc.StoreSession(ctx, userID, session)

	if err := svc.DeleteSession(ctx, userID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err := svc.GetSession(ctx, userID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestSessionTTL(t *testing.T) {
	client := newTestRedisClient(t)
	defer client.Close()

	svc := NewRedisStore(client)
	ctx := context.Background()
	userID := "test-user-session-ttl"

	client.Del(ctx, "session:casdoor:"+userID)

	// Store with RefreshExpiresAt 2 hours from now
	session := &CasdoorSession{
		AccessToken:      "token-ttl",
		RefreshExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
	}
	svc.StoreSession(ctx, userID, session)

	ttl := client.TTL(ctx, "session:casdoor:"+userID).Val()
	if ttl < time.Hour {
		t.Errorf("TTL = %v, should be around 2 hours", ttl)
	}
	if ttl > 3*time.Hour {
		t.Errorf("TTL = %v, should not exceed 3 hours", ttl)
	}
}

func TestStoreSession_Overwrite(t *testing.T) {
	client := newTestRedisClient(t)
	defer client.Close()

	svc := NewRedisStore(client)
	ctx := context.Background()
	userID := "test-user-session-overwrite"

	client.Del(ctx, "session:casdoor:"+userID)

	svc.StoreSession(ctx, userID, &CasdoorSession{
		AccessToken: "old-token",
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	})

	svc.StoreSession(ctx, userID, &CasdoorSession{
		AccessToken: "new-token",
		ExpiresAt:   time.Now().Add(2 * time.Hour).Unix(),
	})

	got, err := svc.GetSession(ctx, userID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.AccessToken != "new-token" {
		t.Errorf("access_token = %q, want %q", got.AccessToken, "new-token")
	}
}
