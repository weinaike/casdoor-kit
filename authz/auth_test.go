package authz

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weinaike/casdoor-kit/authz/casdoor"
	"github.com/weinaike/casdoor-kit/authz/session"
	"github.com/weinaike/casdoor-kit/config"
)

// compile-time check
var _ AuthService = (*authService)(nil)

// --- Mocks ---

type mockCasdoorClient struct {
	loginURL      string
	signupURL     string
	logoutURL     string
	org           string

	exchangeCodeFn    func(ctx context.Context, code string) (*casdoor.TokenResponse, error)
	getUserInfoFn     func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error)
	refreshTokenFn    func(ctx context.Context, refreshToken string) (*casdoor.TokenResponse, error)
	getSystemTokenFn  func(ctx context.Context) (string, error)
	revokeTokenFn     func(ctx context.Context, accessToken string) error
}

func (m *mockCasdoorClient) GetOrganization() string                                { return m.org }
func (m *mockCasdoorClient) GetLoginURL(state string) string                        { return m.loginURL }
func (m *mockCasdoorClient) GetSignupURL() string                                   { return m.signupURL }
func (m *mockCasdoorClient) GetLogoutURL(casdoorAccessToken string) string         { return m.logoutURL }
func (m *mockCasdoorClient) GetProducts(ctx context.Context, accessToken string) ([]casdoor.Product, error) {
	return nil, nil
}
func (m *mockCasdoorClient) GetProduct(ctx context.Context, accessToken string, productName string) (*casdoor.Product, error) {
	return nil, nil
}
func (m *mockCasdoorClient) PlaceOrder(ctx context.Context, accessToken string, productName string) (*casdoor.Order, error) {
	return nil, nil
}
func (m *mockCasdoorClient) PayOrder(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error) {
	return nil, nil
}
func (m *mockCasdoorClient) GetUserOrders(ctx context.Context, accessToken string, userName string) ([]casdoor.Order, error) {
	return nil, nil
}
func (m *mockCasdoorClient) GetOrder(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
	return nil, nil
}
func (m *mockCasdoorClient) CancelOrder(ctx context.Context, orderOwner string, orderName string) error {
	return nil
}
func (m *mockCasdoorClient) LoginByPassword(ctx context.Context, username, password string) (*casdoor.TokenResponse, error) {
	return nil, nil
}
func (m *mockCasdoorClient) NotifyPayment(ctx context.Context, owner string, paymentName string) error {
	return nil
}

func (m *mockCasdoorClient) ExchangeCode(ctx context.Context, code string) (*casdoor.TokenResponse, error) {
	if m.exchangeCodeFn != nil {
		return m.exchangeCodeFn(ctx, code)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCasdoorClient) GetUserInfo(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
	if m.getUserInfoFn != nil {
		return m.getUserInfoFn(ctx, accessToken)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCasdoorClient) RefreshToken(ctx context.Context, refreshToken string) (*casdoor.TokenResponse, error) {
	if m.refreshTokenFn != nil {
		return m.refreshTokenFn(ctx, refreshToken)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCasdoorClient) GetSystemToken(ctx context.Context) (string, error) {
	if m.getSystemTokenFn != nil {
		return m.getSystemTokenFn(ctx)
	}
	return "", fmt.Errorf("not implemented")
}

func (m *mockCasdoorClient) RevokeToken(ctx context.Context, accessToken string) error {
	if m.revokeTokenFn != nil {
		return m.revokeTokenFn(ctx, accessToken)
	}
	return nil
}

type mockSessionService struct {
	sessions map[string]*session.CasdoorSession
	storeErr error
}

func newMockSessionService() *mockSessionService {
	return &mockSessionService{sessions: make(map[string]*session.CasdoorSession)}
}

func (m *mockSessionService) StoreSession(ctx context.Context, userID string, s *session.CasdoorSession) error {
	if m.storeErr != nil {
		return m.storeErr
	}
	m.sessions[userID] = s
	return nil
}

func (m *mockSessionService) GetSession(ctx context.Context, userID string) (*session.CasdoorSession, error) {
	s, ok := m.sessions[userID]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return s, nil
}

func (m *mockSessionService) DeleteSession(ctx context.Context, userID string) error {
	delete(m.sessions, userID)
	return nil
}

// --- Helpers ---

func generateRSAKeys(t *testing.T) (privateKeyPath, publicKeyPath string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	dir := t.TempDir()

	privBytes := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	privateKeyPath = filepath.Join(dir, "private.key")
	if err := os.WriteFile(privateKeyPath, privPEM, 0600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	publicKeyPath = filepath.Join(dir, "public.key")
	if err := os.WriteFile(publicKeyPath, pubPEM, 0644); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	return privateKeyPath, publicKeyPath
}

func newTestAuthService(t *testing.T, mockClient *mockCasdoorClient, sessStore *mockSessionService) AuthService {
	t.Helper()

	privPath, _ := generateRSAKeys(t)

	cfg := &config.JWTConfig{
		PrivateKeyPath: privPath,
		Issuer:         "test-issuer",
		AccessTokenTTL: 3600,
	}

	svc, err := NewAuthService(mockClient, cfg, sessStore)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	return svc
}

// --- Tests ---

func TestGetLoginURL_ContainsParams(t *testing.T) {
	client := &mockCasdoorClient{
		loginURL: "https://casdoor.example.com/login/oauth/authorize?client_id=test-id&state=abc",
	}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	url := svc.GetLoginURL("abc")
	if !strings.Contains(url, "client_id=test-id") {
		t.Errorf("login URL should contain client_id, got: %s", url)
	}
	if !strings.Contains(url, "state=abc") {
		t.Errorf("login URL should contain state, got: %s", url)
	}
}

func TestGetSignupURL(t *testing.T) {
	client := &mockCasdoorClient{
		signupURL: "https://casdoor.example.com/signup/test-app",
	}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	url := svc.GetSignupURL()
	if !strings.Contains(url, "signup") {
		t.Errorf("signup URL should contain 'signup', got: %s", url)
	}
}

func TestGetLogoutURL(t *testing.T) {
	client := &mockCasdoorClient{
		logoutURL: "https://casdoor.example.com/api/logout?id_token_hint=token123",
	}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	url := svc.GetLogoutURL("token123")
	if !strings.Contains(url, "logout") {
		t.Errorf("logout URL should contain 'logout', got: %s", url)
	}
	if !strings.Contains(url, "token123") {
		t.Errorf("logout URL should contain the token, got: %s", url)
	}
}

// --- P3: Boundary and context tests ---

func TestGetCasdoorToken_RefreshBoundary(t *testing.T) {
	// Token expires in exactly tokenRefreshBuffer — should trigger refresh
	refreshCalled := false
	client := &mockCasdoorClient{
		refreshTokenFn: func(ctx context.Context, refreshToken string) (*casdoor.TokenResponse, error) {
			refreshCalled = true
			return &casdoor.TokenResponse{AccessToken: "refreshed", ExpiresIn: 3600}, nil
		},
	}
	sessStore := newMockSessionService()
	// Expires exactly at now + tokenRefreshBuffer → time.Unix(...).After(now+buffer) is false → refresh
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "borderline",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(tokenRefreshBuffer).Unix(),
	}
	svc := newTestAuthService(t, client, sessStore)

	token, err := svc.GetCasdoorToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetCasdoorToken: %v", err)
	}
	if !refreshCalled {
		t.Error("RefreshToken should have been called at the boundary")
	}
	if token != "refreshed" {
		t.Errorf("token = %q, want %q", token, "refreshed")
	}
}

func TestGetCasdoorToken_RefreshResponseEmptyRefreshToken(t *testing.T) {
	// When refresh response has empty RefreshToken, old refresh token should be preserved
	client := &mockCasdoorClient{
		refreshTokenFn: func(ctx context.Context, refreshToken string) (*casdoor.TokenResponse, error) {
			return &casdoor.TokenResponse{
				AccessToken:  "new-access",
				RefreshToken: "", // empty — should preserve old
				ExpiresIn:    3600,
			}, nil
		},
	}
	sessStore := newMockSessionService()
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "expired-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour).Unix(),
	}
	svc := newTestAuthService(t, client, sessStore)

	token, err := svc.GetCasdoorToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetCasdoorToken: %v", err)
	}
	if token != "new-access" {
		t.Errorf("token = %q, want %q", token, "new-access")
	}

	// Old refresh token should be preserved
	sess := sessStore.sessions["user-1"]
	if sess.RefreshToken != "old-refresh" {
		t.Errorf("RefreshToken = %q, want %q (should preserve old)", sess.RefreshToken, "old-refresh")
	}
}

func TestGetCasdoorToken_StoreSessionFailure(t *testing.T) {
	client := &mockCasdoorClient{
		refreshTokenFn: func(ctx context.Context, refreshToken string) (*casdoor.TokenResponse, error) {
			return &casdoor.TokenResponse{
				AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 3600,
			}, nil
		},
	}
	sessStore := newMockSessionService()
	sessStore.storeErr = fmt.Errorf("redis down")
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "expired-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour).Unix(),
	}
	svc := newTestAuthService(t, client, sessStore)

	// Should still return the new token (graceful degradation)
	token, err := svc.GetCasdoorToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("should succeed even if store fails: %v", err)
	}
	if token != "new-access" {
		t.Errorf("token = %q, want %q", token, "new-access")
	}
}

func TestLogout_EmptyAccessToken(t *testing.T) {
	revokeCalled := false
	client := &mockCasdoorClient{
		revokeTokenFn: func(ctx context.Context, accessToken string) error {
			revokeCalled = true
			return nil
		},
	}
	sessStore := newMockSessionService()
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "", // empty — RevokeToken should be skipped
		RefreshToken: "refresh-abc",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}
	svc := newTestAuthService(t, client, sessStore)

	err := svc.Logout(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if revokeCalled {
		t.Error("RevokeToken should NOT be called when AccessToken is empty")
	}
}

func TestGenerateState_Uniqueness(t *testing.T) {
	client := &mockCasdoorClient{}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	states := make(map[string]bool)
	for i := 0; i < 100; i++ {
		state := svc.GenerateState()
		if len(state) != 32 {
			t.Errorf("state should be 32 hex chars, got %d: %s", len(state), state)
		}
		if states[state] {
			t.Errorf("duplicate state generated: %s", state)
		}
		states[state] = true
	}
}

func TestHandleCallback_Success(t *testing.T) {
	client := &mockCasdoorClient{
		exchangeCodeFn: func(ctx context.Context, code string) (*casdoor.TokenResponse, error) {
			if code != "valid-code" {
				return nil, fmt.Errorf("invalid code")
			}
			return &casdoor.TokenResponse{
				AccessToken:  "casdoor-access-token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				RefreshToken: "casdoor-refresh-token",
			}, nil
		},
		getUserInfoFn: func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
			return &casdoor.UserInfo{
				ID:          "user-123",
				Name:        "testuser",
				DisplayName: "Test User",
				Email:       "test@example.com",
				Avatar:      "https://avatar.example.com/test.png",
			}, nil
		},
	}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	result, err := svc.HandleCallback("valid-code")
	if err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}

	if result.Token == "" {
		t.Error("token should not be empty")
	}
	if result.User == nil {
		t.Fatal("user should not be nil")
	}
	if result.User.ID != "user-123" {
		t.Errorf("user ID = %q, want %q", result.User.ID, "user-123")
	}
	if result.User.Email != "test@example.com" {
		t.Errorf("user email = %q, want %q", result.User.Email, "test@example.com")
	}
	if result.User.Name != "testuser" {
		t.Errorf("user name = %q, want %q", result.User.Name, "testuser")
	}
	if result.ExpiresAt.Before(time.Now()) {
		t.Error("expires_at should be in the future")
	}

	// Verify session was stored
	sess, err := sessStore.GetSession(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("session should exist for user-123: %v", err)
	}
	if sess.AccessToken != "casdoor-access-token" {
		t.Errorf("session access token = %q, want %q", sess.AccessToken, "casdoor-access-token")
	}
	if sess.RefreshToken != "casdoor-refresh-token" {
		t.Errorf("session refresh token = %q, want %q", sess.RefreshToken, "casdoor-refresh-token")
	}
}

func TestHandleCallback_ExchangeCodeError(t *testing.T) {
	client := &mockCasdoorClient{
		exchangeCodeFn: func(ctx context.Context, code string) (*casdoor.TokenResponse, error) {
			return nil, fmt.Errorf("invalid authorization code")
		},
	}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	_, err := svc.HandleCallback("bad-code")
	if err == nil {
		t.Fatal("expected error for bad code")
	}
	if !strings.Contains(err.Error(), "换取 token") {
		t.Errorf("error should mention token exchange, got: %v", err)
	}
}

func TestHandleCallback_GetUserInfoError(t *testing.T) {
	client := &mockCasdoorClient{
		exchangeCodeFn: func(ctx context.Context, code string) (*casdoor.TokenResponse, error) {
			return &casdoor.TokenResponse{
				AccessToken: "some-token",
				ExpiresIn:   3600,
			}, nil
		},
		getUserInfoFn: func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
			return nil, fmt.Errorf("server error")
		},
	}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	_, err := svc.HandleCallback("valid-code")
	if err == nil {
		t.Fatal("expected error when GetUserInfo fails")
	}
	if !strings.Contains(err.Error(), "获取用户信息") {
		t.Errorf("error should mention user info, got: %v", err)
	}
}

func TestLogout_Success(t *testing.T) {
	revokeCalled := false
	client := &mockCasdoorClient{
		revokeTokenFn: func(ctx context.Context, accessToken string) error {
			revokeCalled = true
			if accessToken != "access-abc" {
				t.Errorf("revoke called with token = %q, want %q", accessToken, "access-abc")
			}
			return nil
		},
	}
	sessStore := newMockSessionService()
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-abc",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}

	svc := newTestAuthService(t, client, sessStore)

	err := svc.Logout(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if !revokeCalled {
		t.Error("RevokeToken should have been called")
	}

	_, err = sessStore.GetSession(context.Background(), "user-1")
	if err == nil {
		t.Error("session should have been deleted")
	}
}

func TestLogout_NoSession(t *testing.T) {
	client := &mockCasdoorClient{}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	err := svc.Logout(context.Background(), "nonexistent-user")
	if err != nil {
		t.Fatalf("Logout with no session should succeed, got: %v", err)
	}
}

func TestGetCasdoorToken_ValidSession(t *testing.T) {
	client := &mockCasdoorClient{}
	sessStore := newMockSessionService()
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "valid-access-token",
		RefreshToken: "valid-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(), // not expired
	}

	svc := newTestAuthService(t, client, sessStore)

	token, err := svc.GetCasdoorToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetCasdoorToken: %v", err)
	}
	if token != "valid-access-token" {
		t.Errorf("token = %q, want %q", token, "valid-access-token")
	}
}

func TestGetCasdoorToken_RefreshNeeded(t *testing.T) {
	refreshCalled := false
	client := &mockCasdoorClient{
		refreshTokenFn: func(ctx context.Context, refreshToken string) (*casdoor.TokenResponse, error) {
			refreshCalled = true
			if refreshToken != "old-refresh" {
				t.Errorf("refresh called with %q, want %q", refreshToken, "old-refresh")
			}
			return &casdoor.TokenResponse{
				AccessToken:  "new-access",
				RefreshToken: "new-refresh",
				ExpiresIn:    3600,
			}, nil
		},
	}
	sessStore := newMockSessionService()
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "expired-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour).Unix(), // expired
	}

	svc := newTestAuthService(t, client, sessStore)

	token, err := svc.GetCasdoorToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetCasdoorToken: %v", err)
	}

	if token != "new-access" {
		t.Errorf("token = %q, want %q", token, "new-access")
	}
	if !refreshCalled {
		t.Error("RefreshToken should have been called")
	}

	// Verify session was updated
	sess := sessStore.sessions["user-1"]
	if sess.AccessToken != "new-access" {
		t.Errorf("session access token = %q, want %q", sess.AccessToken, "new-access")
	}
	if sess.RefreshToken != "new-refresh" {
		t.Errorf("session refresh token = %q, want %q", sess.RefreshToken, "new-refresh")
	}
}

func TestGetCasdoorToken_NoSession(t *testing.T) {
	client := &mockCasdoorClient{}
	sessStore := newMockSessionService()
	svc := newTestAuthService(t, client, sessStore)

	_, err := svc.GetCasdoorToken(context.Background(), "unknown-user")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !strings.Contains(err.Error(), "会话不存在") {
		t.Errorf("error should mention session not found, got: %v", err)
	}
}

func TestGetCasdoorToken_NoRefreshToken(t *testing.T) {
	client := &mockCasdoorClient{}
	sessStore := newMockSessionService()
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "expired-access",
		RefreshToken: "", // no refresh token
		ExpiresAt:    time.Now().Add(-time.Hour).Unix(),
	}

	svc := newTestAuthService(t, client, sessStore)

	_, err := svc.GetCasdoorToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error when no refresh token available")
	}
	if !strings.Contains(err.Error(), "refresh token") {
		t.Errorf("error should mention refresh token, got: %v", err)
	}
}

func TestGetCasdoorToken_RefreshFailure_DeletesSession(t *testing.T) {
	client := &mockCasdoorClient{
		refreshTokenFn: func(ctx context.Context, refreshToken string) (*casdoor.TokenResponse, error) {
			return nil, fmt.Errorf("refresh failed")
		},
	}
	sessStore := newMockSessionService()
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "expired-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour).Unix(),
	}

	svc := newTestAuthService(t, client, sessStore)

	_, err := svc.GetCasdoorToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error when refresh fails")
	}

	// Session should be deleted on refresh failure
	_, sessErr := sessStore.GetSession(context.Background(), "user-1")
	if sessErr == nil {
		t.Error("session should be deleted after refresh failure")
	}
}

func BenchmarkGetCasdoorToken_ValidSession(b *testing.B) {
	client := &mockCasdoorClient{}
	sessStore := newMockSessionService()
	sessStore.sessions["user-1"] = &session.CasdoorSession{
		AccessToken:  "valid-access-token",
		RefreshToken: "valid-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}

	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	dir := b.TempDir()
	privBytes := x509.MarshalPKCS1PrivateKey(privKey)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	privPath := filepath.Join(dir, "private.key")
	os.WriteFile(privPath, privPEM, 0600)

	cfg := &config.JWTConfig{PrivateKeyPath: privPath, Issuer: "bench", AccessTokenTTL: 3600}
	svc, _ := NewAuthService(client, cfg, sessStore)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.GetCasdoorToken(ctx, "user-1")
	}
}

func BenchmarkHandleCallback(b *testing.B) {
	client := &mockCasdoorClient{
		exchangeCodeFn: func(ctx context.Context, code string) (*casdoor.TokenResponse, error) {
			return &casdoor.TokenResponse{AccessToken: "token", ExpiresIn: 3600, RefreshToken: "refresh"}, nil
		},
		getUserInfoFn: func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
			return &casdoor.UserInfo{ID: "user-1", Name: "test", DisplayName: "Test", Email: "test@test.com"}, nil
		},
	}
	sessStore := newMockSessionService()

	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	dir := b.TempDir()
	privBytes := x509.MarshalPKCS1PrivateKey(privKey)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	privPath := filepath.Join(dir, "private.key")
	os.WriteFile(privPath, privPEM, 0600)

	cfg := &config.JWTConfig{PrivateKeyPath: privPath, Issuer: "bench", AccessTokenTTL: 3600}
	svc, _ := NewAuthService(client, cfg, sessStore)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.HandleCallback("code")
	}
}
