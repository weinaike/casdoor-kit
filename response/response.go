package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the unified HTTP response structure.
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

const (
	CodeSuccess         = 0
	CodeBadRequest      = 400
	CodeUnauthorized    = 401
	CodeForbidden       = 403
	CodeNotFound        = 404
	CodePaymentRequired = 402
	CodeInternalError   = 500
)

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: CodeSuccess, Data: data})
}

func Error(c *gin.Context, httpStatus int, businessCode int, message string) {
	c.JSON(httpStatus, Response{Code: businessCode, Message: message})
}

func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, CodeBadRequest, message)
}

func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, CodeUnauthorized, message)
}

func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, CodeForbidden, message)
}

func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, CodeNotFound, message)
}

func PaymentRequired(c *gin.Context, message string) {
	Error(c, http.StatusPaymentRequired, CodePaymentRequired, message)
}

func InternalError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, CodeInternalError, message)
}

func BindError(c *gin.Context, err error) {
	BadRequest(c, "参数错误: "+err.Error())
}

func SuccessWithStatus(c *gin.Context, httpStatus int, data interface{}) {
	c.JSON(httpStatus, Response{Code: CodeSuccess, Data: data})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{Code: CodeSuccess, Data: data})
}

func SuccessWithTotal(c *gin.Context, data interface{}, total int64) {
	c.JSON(http.StatusOK, Response{
		Code: CodeSuccess,
		Data: map[string]interface{}{"list": data, "total": total},
	})
}
