package handler

import (
	"errors"

	"github.com/weinaike/casdoor-kit/billing"
	"github.com/weinaike/casdoor-kit/middleware"
	"github.com/weinaike/casdoor-kit/response"
	"github.com/gin-gonic/gin"
)

// PaymentHandler handles payment HTTP requests.
type PaymentHandler struct {
	paymentService     billing.PaymentService
	entitlementService billing.EntitlementService
}

// NewPaymentHandler creates a payment handler.
func NewPaymentHandler(paymentService billing.PaymentService, entitlementService billing.EntitlementService) *PaymentHandler {
	return &PaymentHandler{
		paymentService:     paymentService,
		entitlementService: entitlementService,
	}
}

// GetProducts returns the product list.
// GET /api/v1/products
func (h *PaymentHandler) GetProducts(c *gin.Context) {
	userID := middleware.GetUserID(c)

	products, err := h.paymentService.GetProducts(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "获取产品列表失败: "+err.Error())
		return
	}

	response.Success(c, products)
}

// CreateOrderRequest is the request for creating an order.
type CreateOrderRequest struct {
	ProductName string `json:"product_name" binding:"required"`
	Provider    string `json:"provider" binding:"required"`
}

// CreateOrder creates an order and initiates payment.
// POST /api/v1/orders
func (h *PaymentHandler) CreateOrder(c *gin.Context) {
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BindError(c, err)
		return
	}

	userID := middleware.GetUserID(c)

	result, err := h.paymentService.CreateOrder(c.Request.Context(), userID, &billing.CreateOrderInput{
		ProductName: req.ProductName,
		Provider:    req.Provider,
	})
	if err != nil {
		response.InternalError(c, "创建订单失败: "+err.Error())
		return
	}

	response.Success(c, result)
}

// GetOrders returns the user's order history.
// GET /api/v1/orders
func (h *PaymentHandler) GetOrders(c *gin.Context) {
	userID := middleware.GetUserID(c)

	limit := 20
	offset := 0
	if l := c.Query("limit"); l != "" {
		if _, err := parseIntParam(l, &limit); err != nil {
			response.BadRequest(c, "invalid limit")
			return
		}
	}
	if o := c.Query("offset"); o != "" {
		if _, err := parseIntParam(o, &offset); err != nil {
			response.BadRequest(c, "invalid offset")
			return
		}
	}

	orders, total, err := h.paymentService.GetOrders(c.Request.Context(), userID, limit, offset)
	if err != nil {
		response.InternalError(c, "获取订单列表失败: "+err.Error())
		return
	}

	response.SuccessWithTotal(c, orders, total)
}

// CancelOrder cancels an order.
// POST /api/v1/orders/:order_name/cancel
func (h *PaymentHandler) CancelOrder(c *gin.Context) {
	orderName := c.Param("order_name")
	if orderName == "" {
		response.BadRequest(c, "order_name is required")
		return
	}

	userID := middleware.GetUserID(c)

	if err := h.paymentService.CancelOrder(c.Request.Context(), userID, orderName); err != nil {
		response.InternalError(c, "取消订单失败: "+err.Error())
		return
	}

	response.Success(c, map[string]string{"message": "订单已取消"})
}

// PayOrder initiates payment for an existing order.
// POST /api/v1/orders/:order_name/pay
func (h *PaymentHandler) PayOrder(c *gin.Context) {
	orderName := c.Param("order_name")
	if orderName == "" {
		response.BadRequest(c, "order_name is required")
		return
	}

	var req struct {
		Provider string `json:"provider" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BindError(c, err)
		return
	}

	userID := middleware.GetUserID(c)

	result, err := h.paymentService.PayOrder(c.Request.Context(), userID, orderName, req.Provider)
	if err != nil {
		response.InternalError(c, "支付订单失败: "+err.Error())
		return
	}

	response.Success(c, result)
}

// GetBalance returns the user's balance.
// GET /api/v1/balance
func (h *PaymentHandler) GetBalance(c *gin.Context) {
	userID := middleware.GetUserID(c)

	balance, err := h.entitlementService.GetWallet(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, "获取余额失败: "+err.Error())
		return
	}

	response.Success(c, balance)
}

// ListEntitlements returns the user's entitlements.
// GET /api/v1/entitlements
func (h *PaymentHandler) ListEntitlements(c *gin.Context) {
	userID := middleware.GetUserID(c)

	limit := 20
	offset := 0
	if l := c.Query("limit"); l != "" {
		if _, err := parseIntParam(l, &limit); err != nil {
			response.BadRequest(c, "invalid limit")
			return
		}
	}
	if o := c.Query("offset"); o != "" {
		if _, err := parseIntParam(o, &offset); err != nil {
			response.BadRequest(c, "invalid offset")
			return
		}
	}

	entitlements, total, err := h.entitlementService.ListEntitlements(c.Request.Context(), userID, limit, offset)
	if err != nil {
		response.InternalError(c, "获取权益包列表失败: "+err.Error())
		return
	}

	response.SuccessWithTotal(c, entitlements, total)
}

// PaymentCallback handles payment callbacks from Casdoor.
// POST /api/v1/payment/callback
func (h *PaymentHandler) PaymentCallback(c *gin.Context) {
	var orderName string

	transactionOwner := c.Query("transactionOwner")
	transactionName := c.Query("transactionName")
	if transactionOwner != "" && transactionName != "" {
		orderName = transactionName
	} else {
		var req struct {
			OrderName string `json:"order_name" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BindError(c, err)
			return
		}
		orderName = req.OrderName
	}

	if orderName == "" {
		response.BadRequest(c, "order_name or transactionOwner/transactionName is required")
		return
	}

	if err := h.paymentService.HandlePaymentCallback(c.Request.Context(), orderName); err != nil {
		response.InternalError(c, "处理支付回调失败: "+err.Error())
		return
	}

	response.Success(c, map[string]string{"message": "success"})
}

// SyncOrder syncs an order's status.
// POST /api/v1/orders/:order_name/sync
func (h *PaymentHandler) SyncOrder(c *gin.Context) {
	orderName := c.Param("order_name")
	if orderName == "" {
		response.BadRequest(c, "order_name is required")
		return
	}

	userID := middleware.GetUserID(c)

	result, err := h.paymentService.SyncOrder(c.Request.Context(), userID, orderName)
	if err != nil {
		response.InternalError(c, "同步订单失败: "+err.Error())
		return
	}

	response.Success(c, result)
}

// GetBillingHistory returns the billing history.
// GET /api/v1/billing/history
func (h *PaymentHandler) GetBillingHistory(c *gin.Context) {
	userID := middleware.GetUserID(c)

	limit := 20
	offset := 0
	if l := c.Query("limit"); l != "" {
		if _, err := parseIntParam(l, &limit); err != nil {
			response.BadRequest(c, "invalid limit")
			return
		}
	}
	if o := c.Query("offset"); o != "" {
		if _, err := parseIntParam(o, &offset); err != nil {
			response.BadRequest(c, "invalid offset")
			return
		}
	}

	history, total, err := h.entitlementService.GetBillingHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		response.InternalError(c, "获取计费历史失败: "+err.Error())
		return
	}

	response.SuccessWithTotal(c, history, total)
}

func parseIntParam(s string, result *int) (int, error) {
	var val int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid parameter value")
		}
		val = val*10 + int(c-'0')
	}
	*result = val
	return val, nil
}
