package authz

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/weinaike/casdoor-kit"
	"github.com/weinaike/casdoor-kit/authz/casdoor"
	"github.com/weinaike/casdoor-kit/authz/session"
	"github.com/weinaike/casdoor-kit/config"
	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultRefreshTokenExpiry = 7 * 24 * time.Hour
	tokenRefreshBuffer        = 5 * time.Minute
)

// AuthService is the authentication service interface.
type AuthService interface {
	GetLoginURL(state string) string
	GetSignupURL() string
	GetLogoutURL(casdoorAccessToken string) string
	HandleCallback(code string) (*AuthResult, error)
	GenerateState() string
	Logout(ctx context.Context, userID string) error
	GetCasdoorToken(ctx context.Context, userID string) (string, error)
}

// AuthResult is the result of a successful authentication.
type AuthResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      *UserInfo `json:"user"`
}

// UserInfo holds user information.
type UserInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar"`
}

type authService struct {
	casdoorClient  casdoor.ClientInterface
	jwtConfig      *config.JWTConfig
	privateKey     *rsa.PrivateKey
	sessionService session.SessionService
}

// NewAuthService creates an authentication service.
func NewAuthService(casdoorClient casdoor.ClientInterface, jwtConfig *config.JWTConfig, sessionService session.SessionService) (AuthService, error) {
	keyBytes, err := os.ReadFile(jwtConfig.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("读取 JWT 私钥失败: %w", err)
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("解析 JWT 私钥失败: %w", err)
	}

	return &authService{
		casdoorClient:  casdoorClient,
		jwtConfig:      jwtConfig,
		privateKey:     privateKey,
		sessionService: sessionService,
	}, nil
}

func (s *authService) GetLoginURL(state string) string {
	return s.casdoorClient.GetLoginURL(state)
}

func (s *authService) GetSignupURL() string {
	return s.casdoorClient.GetSignupURL()
}

func (s *authService) GetLogoutURL(casdoorAccessToken string) string {
	return s.casdoorClient.GetLogoutURL(casdoorAccessToken)
}

func (s *authService) GenerateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *authService) HandleCallback(code string) (*AuthResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokenResp, err := s.casdoorClient.ExchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("换取 token 失败: %w", err)
	}

	casdoorUser, err := s.casdoorClient.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	now := time.Now()
	sess := &session.CasdoorSession{
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     tokenResp.RefreshToken,
		ExpiresAt:        now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix(),
		RefreshExpiresAt: now.Add(defaultRefreshTokenExpiry).Unix(),
	}

	if err := s.sessionService.StoreSession(ctx, casdoorUser.ID, sess); err != nil {
		gokit.GetLogger().Warn("存储 Casdoor 会话失败", "error", err)
	}

	expiresAt := now.Add(time.Duration(s.jwtConfig.AccessTokenTTL) * time.Second)
	claims := jwt.MapClaims{
		"user_id": casdoorUser.ID,
		"email":   casdoorUser.Email,
		"name":    casdoorUser.Name,
		"iss":     s.jwtConfig.Issuer,
		"iat":     now.Unix(),
		"exp":     expiresAt.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("签名 token 失败: %w", err)
	}

	gokit.GetLogger().Info("用户认证成功",
		"user_id", casdoorUser.ID,
		"email", casdoorUser.Email)

	return &AuthResult{
		Token:     tokenString,
		ExpiresAt: expiresAt,
		User: &UserInfo{
			ID:          casdoorUser.ID,
			Name:        casdoorUser.Name,
			DisplayName: casdoorUser.DisplayName,
			Email:       casdoorUser.Email,
			Avatar:      casdoorUser.Avatar,
		},
	}, nil
}

func (s *authService) Logout(ctx context.Context, userID string) error {
	sess, err := s.sessionService.GetSession(ctx, userID)
	if err != nil {
		gokit.GetLogger().Debug("登出时会话未找到", "error", err)
	}

	if sess != nil && sess.AccessToken != "" {
		if err := s.casdoorClient.RevokeToken(ctx, sess.AccessToken); err != nil {
			gokit.GetLogger().Warn("撤销 Casdoor token 失败", "error", err)
		}
	}

	if err := s.sessionService.DeleteSession(ctx, userID); err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}

	gokit.GetLogger().Info("用户登出成功", "user_id", userID)
	return nil
}

func (s *authService) GetCasdoorToken(ctx context.Context, userID string) (string, error) {
	sess, err := s.sessionService.GetSession(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("会话不存在: %w", err)
	}

	now := time.Now()
	if time.Unix(sess.ExpiresAt, 0).After(now.Add(tokenRefreshBuffer)) {
		return sess.AccessToken, nil
	}

	if sess.RefreshToken == "" {
		return "", errors.New("无 refresh token，需要重新登录")
	}

	newTokenResp, err := s.casdoorClient.RefreshToken(ctx, sess.RefreshToken)
	if err != nil {
		s.sessionService.DeleteSession(ctx, userID)
		return "", fmt.Errorf("token 刷新失败: %w", err)
	}

	sess.AccessToken = newTokenResp.AccessToken
	if newTokenResp.RefreshToken != "" {
		sess.RefreshToken = newTokenResp.RefreshToken
	}
	sess.ExpiresAt = now.Add(time.Duration(newTokenResp.ExpiresIn) * time.Second).Unix()

	if err := s.sessionService.StoreSession(ctx, userID, sess); err != nil {
		gokit.GetLogger().Warn("刷新 token 后更新会话失败", "error", err)
	}

	return sess.AccessToken, nil
}
