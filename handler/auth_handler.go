package handler

import (
	"github.com/weinaike/casdoor-kit/authz"
	"github.com/weinaike/casdoor-kit/middleware"
	"github.com/weinaike/casdoor-kit/response"
	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication HTTP requests.
type AuthHandler struct {
	authService authz.AuthService
}

// NewAuthHandler creates an auth handler.
func NewAuthHandler(authService authz.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// LoginURLResponse is the response for the login URL endpoint.
type LoginURLResponse struct {
	LoginURL  string `json:"login_url"`
	SignupURL string `json:"signup_url"`
	State     string `json:"state"`
}

// CallbackRequest is the OAuth2 callback request.
type CallbackRequest struct {
	Code  string `form:"code" binding:"required"`
	State string `form:"state"`
}

// CallbackResponse is the callback response.
type CallbackResponse struct {
	Token     string         `json:"token"`
	ExpiresAt int64          `json:"expires_at"`
	User      *authz.UserInfo `json:"user"`
}

// GetLoginURL returns the Casdoor login URL.
// GET /api/v1/auth/login-url
func (h *AuthHandler) GetLoginURL(c *gin.Context) {
	state := h.authService.GenerateState()
	loginURL := h.authService.GetLoginURL(state)
	signupURL := h.authService.GetSignupURL()

	response.Success(c, LoginURLResponse{
		LoginURL:  loginURL,
		SignupURL: signupURL,
		State:     state,
	})
}

// Callback handles the OAuth2 callback.
// GET/POST /api/v1/auth/callback
func (h *AuthHandler) Callback(c *gin.Context) {
	var req CallbackRequest

	if err := c.ShouldBindQuery(&req); err == nil && req.Code != "" {
		// use query parameters
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BindError(c, err)
			return
		}
	}

	result, err := h.authService.HandleCallback(req.Code)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}

	response.Success(c, CallbackResponse{
		Token:     result.Token,
		ExpiresAt: result.ExpiresAt.Unix(),
		User:      result.User,
	})
}

// GetCurrentUser returns the current user info.
// GET /api/v1/auth/me
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "未认证")
		return
	}
	response.Success(c, gin.H{"user_id": userID})
}

// Logout logs out the user.
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		response.Unauthorized(c, "未认证")
		return
	}

	casdoorToken, err := h.authService.GetCasdoorToken(c.Request.Context(), userID)
	if err != nil {
		casdoorToken = ""
	}

	casdoorLogoutURL := h.authService.GetLogoutURL(casdoorToken)

	if err := h.authService.Logout(c.Request.Context(), userID); err != nil {
		response.InternalError(c, "登出失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"message":            "登出成功",
		"casdoor_logout_url": casdoorLogoutURL,
	})
}
