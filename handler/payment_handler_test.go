package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/weinaike/casdoor-kit/billing"
	"github.com/weinaike/casdoor-kit/billing/model"
	"github.com/weinaike/casdoor-kit/middleware"
	"github.com/gin-gonic/gin"
)

// mockPaymentService implements billing.PaymentService for testing.
type mockPaymentService struct {
	products    []billing.ProductWithEntitlement
	productsErr error
	orderResult *billing.PaymentResult
	orderErr    error
	orders      []billing.OrderHistory
	ordersTotal int64
	ordersErr   error
	cancelErr   error
	payResult   *billing.PaymentResult
	payErr      error
	callbackErr error
	syncResult  *billing.OrderSyncResult
	syncErr     error
}

func (m *mockPaymentService) GetProducts(ctx context.Context, userID string) ([]billing.ProductWithEntitlement, error) {
	return m.products, m.productsErr
}
func (m *mockPaymentService) CreateOrder(ctx context.Context, userID string, req *billing.CreateOrderInput) (*billing.PaymentResult, error) {
	return m.orderResult, m.orderErr
}
func (m *mockPaymentService) GetOrders(ctx context.Context, userID string, limit, offset int) ([]billing.OrderHistory, int64, error) {
	return m.orders, m.ordersTotal, m.ordersErr
}
func (m *mockPaymentService) CancelOrder(ctx context.Context, userID string, orderName string) error {
	return m.cancelErr
}
func (m *mockPaymentService) PayOrder(ctx context.Context, userID string, orderName string, provider string) (*billing.PaymentResult, error) {
	return m.payResult, m.payErr
}
func (m *mockPaymentService) HandlePaymentCallback(ctx context.Context, orderName string) error {
	return m.callbackErr
}
func (m *mockPaymentService) SyncOrder(ctx context.Context, userID string, orderName string) (*billing.OrderSyncResult, error) {
	return m.syncResult, m.syncErr
}

// mockEntitlementSvc implements billing.EntitlementService for testing.
type mockEntitlementSvc struct {
	wallet       *billing.UserWalletInfo
	walletErr    error
	entitlements []billing.EntitlementInfo
	entTotal     int64
	entErr       error
	history      []billing.BillingHistoryEntry
	historyTotal int64
	historyErr   error
}

func (m *mockEntitlementSvc) GetWallet(ctx context.Context, userID string) (*billing.UserWalletInfo, error) {
	return m.wallet, m.walletErr
}
func (m *mockEntitlementSvc) ListEntitlements(ctx context.Context, userID string, limit, offset int) ([]billing.EntitlementInfo, int64, error) {
	return m.entitlements, m.entTotal, m.entErr
}
func (m *mockEntitlementSvc) FreezeForTask(ctx context.Context, userID, taskRef string, seconds int64) error {
	return nil
}
func (m *mockEntitlementSvc) ConsumeTask(ctx context.Context, taskRef string) error {
	return nil
}
func (m *mockEntitlementSvc) UnfreezeTask(ctx context.Context, taskRef string) error {
	return nil
}
func (m *mockEntitlementSvc) GrantEntitlement(ctx context.Context, userID string, productName string, orderID int64) (*model.UserEntitlement, error) {
	return nil, nil
}
func (m *mockEntitlementSvc) GetBillingHistory(ctx context.Context, userID string, limit, offset int) ([]billing.BillingHistoryEntry, int64, error) {
	return m.history, m.historyTotal, m.historyErr
}

func setupPaymentHandler(ps billing.PaymentService, es billing.EntitlementService) *PaymentHandler {
	return NewPaymentHandler(ps, es)
}

func setUserID(c *gin.Context, userID string) {
	c.Set(string(middleware.UserIDKey), userID)
}

func TestGetProducts(t *testing.T) {
	ps := &mockPaymentService{
		products: []billing.ProductWithEntitlement{
			{Name: "basic-pack", DisplayName: "基础包", Price: 29.9, Currency: "CNY"},
		},
	}
	h := setupPaymentHandler(ps, &mockEntitlementSvc{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/products", nil)
	setUserID(c, "user1")
	h.GetProducts(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []billing.ProductWithEntitlement `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].Name != "basic-pack" {
		t.Errorf("name = %q, want %q", resp.Data[0].Name, "basic-pack")
	}
	if resp.Data[0].DisplayName != "基础包" {
		t.Errorf("displayName = %q, want %q", resp.Data[0].DisplayName, "基础包")
	}
	if resp.Data[0].Price != 29.9 {
		t.Errorf("price = %.2f, want 29.9", resp.Data[0].Price)
	}
	if resp.Data[0].Currency != "CNY" {
		t.Errorf("currency = %q, want %q", resp.Data[0].Currency, "CNY")
	}
}

func TestCreateOrder_ValidRequest(t *testing.T) {
	ps := &mockPaymentService{
		orderResult: &billing.PaymentResult{OrderID: "order-1", PaymentURL: "https://pay.test/1", Amount: 29.9, Currency: "CNY"},
	}
	h := setupPaymentHandler(ps, &mockEntitlementSvc{})

	body, _ := json.Marshal(map[string]string{
		"product_name": "basic-pack",
		"provider":     "alipay",
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/orders", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setUserID(c, "user1")
	h.CreateOrder(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			OrderID     string  `json:"order_id"`
			PaymentURL  string  `json:"payment_url"`
			Amount      float64 `json:"amount"`
			Currency    string  `json:"currency"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.OrderID != "order-1" {
		t.Errorf("order_id = %q, want %q", resp.Data.OrderID, "order-1")
	}
	if resp.Data.PaymentURL != "https://pay.test/1" {
		t.Errorf("payment_url = %q, want pay URL", resp.Data.PaymentURL)
	}
	if resp.Data.Amount != 29.9 {
		t.Errorf("amount = %.2f, want 29.9", resp.Data.Amount)
	}
	if resp.Data.Currency != "CNY" {
		t.Errorf("currency = %q, want %q", resp.Data.Currency, "CNY")
	}
}

func TestCreateOrder_MissingFields(t *testing.T) {
	h := setupPaymentHandler(&mockPaymentService{}, &mockEntitlementSvc{})

	body, _ := json.Marshal(map[string]string{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/orders", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setUserID(c, "user1")
	h.CreateOrder(c)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetOrders_DefaultPagination(t *testing.T) {
	ps := &mockPaymentService{
		orders: []billing.OrderHistory{
			{OrderID: "order-1", Price: 10, Currency: "CNY", Status: "Created"},
		},
		ordersTotal: 1,
	}
	h := setupPaymentHandler(ps, &mockEntitlementSvc{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/orders", nil)
	setUserID(c, "user1")
	h.GetOrders(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			List  []billing.OrderHistory `json:"list"`
			Total int64                  `json:"total"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Data.Total)
	}
	if len(resp.Data.List) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(resp.Data.List))
	}
	if resp.Data.List[0].OrderID != "order-1" {
		t.Errorf("order_id = %q, want %q", resp.Data.List[0].OrderID, "order-1")
	}
	if resp.Data.List[0].Price != 10 {
		t.Errorf("price = %.2f, want 10", resp.Data.List[0].Price)
	}
}

func TestGetOrders_InvalidLimit(t *testing.T) {
	h := setupPaymentHandler(&mockPaymentService{}, &mockEntitlementSvc{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/orders?limit=abc", nil)
	setUserID(c, "user1")
	h.GetOrders(c)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetBalance(t *testing.T) {
	es := &mockEntitlementSvc{
		wallet: &billing.UserWalletInfo{
			TotalSeconds:     3600,
			FrozenSeconds:    600,
			AvailableSeconds: 3000,
		},
	}
	h := setupPaymentHandler(&mockPaymentService{}, es)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/balance", nil)
	setUserID(c, "user1")
	h.GetBalance(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			TotalSeconds     int64 `json:"total_seconds"`
			AvailableSeconds int64 `json:"available_seconds"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.AvailableSeconds != 3000 {
		t.Errorf("available_seconds = %d, want 3000", resp.Data.AvailableSeconds)
	}
}

func TestPaymentCallback_QueryParams(t *testing.T) {
	h := setupPaymentHandler(&mockPaymentService{}, &mockEntitlementSvc{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/payment/callback?transactionOwner=admin&transactionName=order-1", nil)
	h.PaymentCallback(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPaymentCallback_JSONBody(t *testing.T) {
	h := setupPaymentHandler(&mockPaymentService{}, &mockEntitlementSvc{})

	body, _ := json.Marshal(map[string]string{"order_name": "order-2"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/payment/callback", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.PaymentCallback(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestPaymentCallback_MissingOrderName(t *testing.T) {
	h := setupPaymentHandler(&mockPaymentService{}, &mockEntitlementSvc{})

	body, _ := json.Marshal(map[string]string{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/payment/callback", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.PaymentCallback(c)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListEntitlements(t *testing.T) {
	es := &mockEntitlementSvc{
		entitlements: []billing.EntitlementInfo{
			{ID: 1, DisplayName: "基础包", TotalSeconds: 3600},
		},
		entTotal: 1,
	}
	h := setupPaymentHandler(&mockPaymentService{}, es)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/entitlements?limit=10&offset=0", nil)
	setUserID(c, "user1")
	h.ListEntitlements(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			List  []billing.EntitlementInfo `json:"list"`
			Total int64                     `json:"total"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Data.Total)
	}
	if len(resp.Data.List) != 1 {
		t.Errorf("list len = %d, want 1", len(resp.Data.List))
	}
}

func TestCancelOrder(t *testing.T) {
	h := setupPaymentHandler(&mockPaymentService{}, &mockEntitlementSvc{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/orders/order-1/cancel", nil)
	c.Params = gin.Params{{Key: "order_name", Value: "order-1"}}
	setUserID(c, "user1")
	h.CancelOrder(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Message != "订单已取消" {
		t.Errorf("message = %q, want %q", resp.Data.Message, "订单已取消")
	}
}

func TestSyncOrder(t *testing.T) {
	ps := &mockPaymentService{
		syncResult: &billing.OrderSyncResult{Message: "同步成功", OrderStatus: "Paid", QuotaSeconds: 3600},
	}
	h := setupPaymentHandler(ps, &mockEntitlementSvc{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/orders/order-1/sync", nil)
	c.Params = gin.Params{{Key: "order_name", Value: "order-1"}}
	setUserID(c, "user1")
	h.SyncOrder(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Message     string `json:"message"`
			OrderStatus string `json:"order_status"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Message != "同步成功" {
		t.Errorf("message = %q, want %q", resp.Data.Message, "同步成功")
	}
	if resp.Data.OrderStatus != "Paid" {
		t.Errorf("order_status = %q, want %q", resp.Data.OrderStatus, "Paid")
	}
}

func TestGetBillingHistory(t *testing.T) {
	es := &mockEntitlementSvc{
		history: []billing.BillingHistoryEntry{
			{ID: 1, ActionType: "FREEZE", AmountSeconds: 120},
		},
		historyTotal: 1,
	}
	h := setupPaymentHandler(&mockPaymentService{}, es)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/billing/history", nil)
	setUserID(c, "user1")
	h.GetBillingHistory(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			List  []billing.BillingHistoryEntry `json:"list"`
			Total int64                         `json:"total"`
		} `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Data.Total)
	}
	if len(resp.Data.List) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(resp.Data.List))
	}
	if resp.Data.List[0].ID != 1 {
		t.Errorf("id = %d, want 1", resp.Data.List[0].ID)
	}
	if resp.Data.List[0].ActionType != "FREEZE" {
		t.Errorf("action_type = %q, want %q", resp.Data.List[0].ActionType, "FREEZE")
	}
	if resp.Data.List[0].AmountSeconds != 120 {
		t.Errorf("amount_seconds = %d, want 120", resp.Data.List[0].AmountSeconds)
	}
}

func TestParseIntParam_Valid(t *testing.T) {
	var result int
	val, err := parseIntParam("42", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 || result != 42 {
		t.Errorf("val=%d, result=%d, want 42", val, result)
	}
}

func TestParseIntParam_InvalidChars(t *testing.T) {
	var result int
	_, err := parseIntParam("12a3", &result)
	if err == nil {
		t.Error("expected error for invalid chars")
	}
}

func TestParseIntParam_Empty(t *testing.T) {
	var result int
	val, err := parseIntParam("", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 0 {
		t.Errorf("val = %d, want 0", val)
	}
}
