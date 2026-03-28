package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/weinaike/casdoor-kit/authz"
	"github.com/weinaike/casdoor-kit/middleware"
	"github.com/gin-gonic/gin"
)

// mockAuthService implements authz.AuthService for testing.
type mockAuthService struct {
	loginURL  string
	signupURL string
	logoutURL string
	state     string
	result    *authz.AuthResult
	callback  func(code string) (*authz.AuthResult, error)
	logoutErr error
	token     string
	tokenErr  error
}

func (m *mockAuthService) GetLoginURL(state string) string {
	if m.loginURL != "" {
		return m.loginURL
	}
	return "https://casdoor.example.com/login?state=" + state
}
func (m *mockAuthService) GetSignupURL() string {
	return m.signupURL
}
func (m *mockAuthService) GetLogoutURL(casdoorAccessToken string) string {
	return m.logoutURL
}
func (m *mockAuthService) HandleCallback(code string) (*authz.AuthResult, error) {
	if m.callback != nil {
		return m.callback(code)
	}
	if m.result != nil {
		return m.result, nil
	}
	return nil, nil
}
func (m *mockAuthService) GenerateState() string {
	if m.state != "" {
		return m.state
	}
	return "test-state-12345"
}
func (m *mockAuthService) Logout(ctx context.Context, userID string) error {
	return m.logoutErr
}
func (m *mockAuthService) GetCasdoorToken(ctx context.Context, userID string) (string, error) {
	return m.token, m.tokenErr
}

func setupAuthRouter(svc *mockAuthService) (*gin.Engine, *AuthHandler) {
	h := NewAuthHandler(svc)
	r := gin.New()
	return r, h
}

func TestGetLoginURL(t *testing.T) {
	svc := &mockAuthService{
		loginURL:  "https://casdoor.test/login",
		signupURL: "https://casdoor.test/signup",
		state:     "abc123",
	}
	r, h := setupAuthRouter(svc)
	r.GET("/auth/login-url", h.GetLoginURL)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/login-url", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			LoginURL  string `json:"login_url"`
			SignupURL string `json:"signup_url"`
			State     string `json:"state"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Data.LoginURL != "https://casdoor.test/login" {
		t.Errorf("login_url = %q", resp.Data.LoginURL)
	}
	if resp.Data.SignupURL != "https://casdoor.test/signup" {
		t.Errorf("signup_url = %q", resp.Data.SignupURL)
	}
	if resp.Data.State != "abc123" {
		t.Errorf("state = %q", resp.Data.State)
	}
}

func TestCallback_QueryParams(t *testing.T) {
	svc := &mockAuthService{
		result: &authz.AuthResult{
			Token:     "jwt-token-xyz",
			ExpiresAt: time.Now().Add(time.Hour),
			User:      &authz.UserInfo{ID: "user1", Name: "Test"},
		},
	}
	r, h := setupAuthRouter(svc)
	r.GET("/auth/callback", h.Callback)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/callback?code=abc&state=xyz", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Token     string `json:"token"`
			ExpiresAt int64  `json:"expires_at"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Data.Token != "jwt-token-xyz" {
		t.Errorf("token = %q", resp.Data.Token)
	}
}

func TestCallback_JSONBody(t *testing.T) {
	svc := &mockAuthService{
		result: &authz.AuthResult{
			Token:     "jwt-from-json",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	r, h := setupAuthRouter(svc)
	r.POST("/auth/callback", h.Callback)

	body, _ := json.Marshal(map[string]string{"code": "json-code"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/callback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCallback_MissingCode(t *testing.T) {
	svc := &mockAuthService{}
	r, h := setupAuthRouter(svc)
	r.POST("/auth/callback", h.Callback)

	body, _ := json.Marshal(map[string]string{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/callback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCallback_AuthFails(t *testing.T) {
	svc := &mockAuthService{
		callback: func(code string) (*authz.AuthResult, error) {
			return nil, context.DeadlineExceeded
		},
	}
	r, h := setupAuthRouter(svc)
	r.GET("/auth/callback", h.Callback)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/callback?code=bad-code", nil)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGetCurrentUser_Authenticated(t *testing.T) {
	svc := &mockAuthService{}
	r, h := setupAuthRouter(svc)
	r.GET("/auth/me", h.GetCurrentUser)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/me", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(string(middleware.UserIDKey), "user-42")
	h.GetCurrentUser(c)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestGetCurrentUser_NotAuthenticated(t *testing.T) {
	svc := &mockAuthService{}
	r, h := setupAuthRouter(svc)
	r.GET("/auth/me", h.GetCurrentUser)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/me", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.GetCurrentUser(c)

	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestLogout_Success(t *testing.T) {
	svc := &mockAuthService{
		token:     "casdoor-token",
		logoutURL: "https://casdoor.test/logout?token=casdoor-token",
	}
	r, h := setupAuthRouter(svc)
	r.POST("/auth/logout", h.Logout)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set(string(middleware.UserIDKey), "user-1")
	h.Logout(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Message          string `json:"message"`
			CasdoorLogoutURL string `json:"casdoor_logout_url"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Data.CasdoorLogoutURL != "https://casdoor.test/logout?token=casdoor-token" {
		t.Errorf("casdoor_logout_url = %q", resp.Data.CasdoorLogoutURL)
	}
}

func TestLogout_AuthFails(t *testing.T) {
	svc := &mockAuthService{
		logoutErr: context.DeadlineExceeded,
	}
	r, h := setupAuthRouter(svc)
	r.POST("/auth/logout", h.Logout)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	c.Request = req
	c.Set(string(middleware.UserIDKey), "user-1")
	h.Logout(c)

	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
