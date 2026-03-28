package middleware

import (
	"github.com/weinaike/casdoor-kit/response"
	"github.com/gin-gonic/gin"
)

// RequireAuth requires the user to be authenticated.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		if userID == "" {
			response.Unauthorized(c, "未认证")
			c.Abort()
			return
		}
		c.Next()
	}
}

// OptionalAuth does not require authentication.
func OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
