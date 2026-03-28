package casdoor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/weinaike/casdoor-kit"
	"github.com/weinaike/casdoor-kit/config"
)

// Client is a Casdoor OAuth2 client.
type Client struct {
	cfg        *config.CasdoorConfig
	httpClient *http.Client
}

// NewClient creates a Casdoor client.
func NewClient(cfg *config.CasdoorConfig) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetOrganization returns the configured organization name.
func (c *Client) GetOrganization() string {
	return c.cfg.Organization
}

// GetLoginURL generates a login URL.
func (c *Client) GetLoginURL(state string) string {
	params := url.Values{}
	params.Set("client_id", c.cfg.ClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", c.cfg.RedirectURI)
	params.Set("scope", "openid profile email")
	params.Set("state", state)
	return fmt.Sprintf("%s/login/oauth/authorize?%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		params.Encode())
}

// GetSignupURL generates a signup URL.
func (c *Client) GetSignupURL() string {
	return fmt.Sprintf("%s/signup/%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		c.cfg.Application)
}

// GetLogoutURL generates a logout URL for RP-initiated logout.
func (c *Client) GetLogoutURL(casdoorAccessToken string) string {
	logoutRedirect := strings.TrimSuffix(c.cfg.RedirectURI, "/auth/callback") + "/login"
	params := url.Values{}
	params.Set("id_token_hint", casdoorAccessToken)
	params.Set("post_logout_redirect_uri", logoutRedirect)
	return fmt.Sprintf("%s/api/logout?%s",
		strings.TrimSuffix(c.cfg.Endpoint, "/"),
		params.Encode())
}

// ExchangeCode exchanges an authorization code for an access token.
func (c *Client) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	tokenURL := fmt.Sprintf("%s/api/login/oauth/access_token",
		strings.TrimSuffix(c.cfg.Endpoint, "/"))

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", c.cfg.ClientID)
	data.Set("client_secret", c.cfg.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", c.cfg.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("Casdoor 返回错误状态: %d, %s", resp.StatusCode, errResp.Error)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	gokit.GetLogger().Info("获取 Casdoor access_token 成功",
		"token_type", tokenResp.TokenType)

	return &tokenResp, nil
}

// GetUserInfo fetches user info from Casdoor.
func (c *Client) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	userInfoURL := fmt.Sprintf("%s/api/userinfo",
		strings.TrimSuffix(c.cfg.Endpoint, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("获取用户信息失败: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var userInfo UserInfo
	if err := json.Unmarshal(bodyBytes, &userInfo); err != nil {
		return nil, fmt.Errorf("解析用户信息失败: %w", err)
	}

	gokit.GetLogger().Info("获取用户信息成功",
		"user_id", userInfo.ID,
		"email", userInfo.Email)

	return &userInfo, nil
}

// RefreshToken refreshes an access token using a refresh token.
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	tokenURL := fmt.Sprintf("%s/api/login/oauth/refresh_token",
		strings.TrimSuffix(c.cfg.Endpoint, "/"))

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", c.cfg.ClientID)
	data.Set("client_secret", c.cfg.ClientSecret)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("刷新令牌失败: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	gokit.GetLogger().Info("刷新 Casdoor token 成功")
	return &tokenResp, nil
}

// GetSystemToken obtains a system-level token using client credentials.
func (c *Client) GetSystemToken(ctx context.Context) (string, error) {
	tokenURL := fmt.Sprintf("%s/api/login/oauth/access_token",
		strings.TrimSuffix(c.cfg.Endpoint, "/"))

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.cfg.ClientID)
	data.Set("client_secret", c.cfg.ClientSecret)
	data.Set("scope", "openid profile email")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("获取系统 token 失败: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// RevokeToken revokes a Casdoor access token.
func (c *Client) RevokeToken(ctx context.Context, accessToken string) error {
	revokeURL := fmt.Sprintf("%s/api/login/oauth/logout",
		strings.TrimSuffix(c.cfg.Endpoint, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		gokit.GetLogger().Warn("Casdoor 令牌撤销失败", "status", resp.StatusCode)
	}

	gokit.GetLogger().Info("Casdoor 令牌撤销成功")
	return nil
}

// GetSubscription fetches user subscription info from Casdoor.
func (c *Client) GetSubscription(ctx context.Context, accessToken string) (*SubscriptionInfo, error) {
	subURL := fmt.Sprintf("%s/api/get-subscription",
		strings.TrimSuffix(c.cfg.Endpoint, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("获取订阅信息失败: status %d", resp.StatusCode)
	}

	var subInfo SubscriptionInfo
	if err := json.NewDecoder(resp.Body).Decode(&subInfo); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &subInfo, nil
}

// GetPaymentHistory fetches user payment history from Casdoor.
func (c *Client) GetPaymentHistory(ctx context.Context, accessToken string) ([]PaymentRecord, error) {
	paymentURL := fmt.Sprintf("%s/api/get-payments",
		strings.TrimSuffix(c.cfg.Endpoint, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, paymentURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("获取支付历史失败: status %d", resp.StatusCode)
	}

	var payments []PaymentRecord
	if err := json.NewDecoder(resp.Body).Decode(&payments); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return payments, nil
}
