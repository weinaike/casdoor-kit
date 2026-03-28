package middleware

import (
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/weinaike/casdoor-kit"
	"github.com/weinaike/casdoor-kit/response"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ContextKey is the context key type.
type ContextKey string

const UserIDKey ContextKey = "user_id"

// JWTClaims holds JWT claims.
type JWTClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

var (
	publicKey     interface{}
	publicKeyOnce sync.Once
	publicKeyErr  error
)

// JWTAuth returns a Gin middleware for JWT authentication.
func JWTAuth(publicKeyPath string) gin.HandlerFunc {
	publicKeyOnce.Do(func() {
		publicKey, publicKeyErr = loadPublicKey(publicKeyPath)
		if publicKeyErr != nil {
			gokit.GetLogger().Error("加载 JWT 公钥失败", "error", publicKeyErr)
		}
	})

	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "缺少认证令牌")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Unauthorized(c, "认证令牌格式错误")
			c.Abort()
			return
		}

		userID, err := parseToken(parts[1])
		if err != nil {
			gokit.GetLogger().Debug("JWT 解析失败", "error", err)
			response.Unauthorized(c, "认证令牌无效或已过期")
			c.Abort()
			return
		}

		c.Set(string(UserIDKey), userID)
		c.Next()
	}
}

func parseToken(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return publicKey, nil
	})
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", errors.New("token invalid")
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	return claims.UserID, nil
}

// ParseTokenFromQueryString parses a JWT token from a query string (for SSE).
func ParseTokenFromQueryString(tokenString string) (string, error) {
	return parseToken(tokenString)
}

// GetUserID extracts the user ID from the Gin context.
func GetUserID(c *gin.Context) string {
	userID, exists := c.Get(string(UserIDKey))
	if !exists {
		return ""
	}
	return userID.(string)
}

func loadPublicKey(path string) (interface{}, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.New("JWT 公钥文件不存在: " + path)
	}
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jwt.ParseRSAPublicKeyFromPEM(keyBytes)
}
