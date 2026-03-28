package billing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/weinaike/casdoor-kit/authz"
	"github.com/weinaike/casdoor-kit/authz/casdoor"
	"github.com/weinaike/casdoor-kit/billing/model"
)

// --- Payment-specific mocks ---

type mockAuthForPayment struct {
	token string
	err   error
}

func (m *mockAuthForPayment) GetLoginURL(state string) string                        { return "" }
func (m *mockAuthForPayment) GetSignupURL() string                                   { return "" }
func (m *mockAuthForPayment) GetLogoutURL(casdoorAccessToken string) string         { return "" }
func (m *mockAuthForPayment) HandleCallback(code string) (*authz.AuthResult, error)  { return nil, nil }
func (m *mockAuthForPayment) GenerateState() string                                  { return "" }
func (m *mockAuthForPayment) Logout(ctx context.Context, userID string) error        { return nil }
func (m *mockAuthForPayment) GetCasdoorToken(ctx context.Context, userID string) (string, error) {
	return m.token, m.err
}

type mockCasdoorForPayment struct {
	getProductsFn    func(ctx context.Context, accessToken string) ([]casdoor.Product, error)
	getUserInfoFn    func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error)
	placeOrderFn     func(ctx context.Context, accessToken string, productName string) (*casdoor.Order, error)
	payOrderFn       func(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error)
	getOrderFn       func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error)
	getUserOrdersFn  func(ctx context.Context, accessToken string, userName string) ([]casdoor.Order, error)
	cancelOrderFn    func(ctx context.Context, orderOwner string, orderName string) error
}

func (m *mockCasdoorForPayment) GetOrganization() string                        { return "test-org" }
func (m *mockCasdoorForPayment) GetLoginURL(state string) string               { return "" }
func (m *mockCasdoorForPayment) GetSignupURL() string                          { return "" }
func (m *mockCasdoorForPayment) GetLogoutURL(casdoorAccessToken string) string { return "" }
func (m *mockCasdoorForPayment) ExchangeCode(ctx context.Context, code string) (*casdoor.TokenResponse, error) {
	return nil, nil
}
func (m *mockCasdoorForPayment) RefreshToken(ctx context.Context, refreshToken string) (*casdoor.TokenResponse, error) {
	return nil, nil
}
func (m *mockCasdoorForPayment) GetSystemToken(ctx context.Context) (string, error)  { return "", nil }
func (m *mockCasdoorForPayment) RevokeToken(ctx context.Context, accessToken string) error { return nil }
func (m *mockCasdoorForPayment) GetProduct(ctx context.Context, accessToken string, productName string) (*casdoor.Product, error) {
	return nil, nil
}
func (m *mockCasdoorForPayment) LoginByPassword(ctx context.Context, username, password string) (*casdoor.TokenResponse, error) {
	return nil, nil
}
func (m *mockCasdoorForPayment) NotifyPayment(ctx context.Context, owner string, paymentName string) error {
	return nil
}

func (m *mockCasdoorForPayment) GetProducts(ctx context.Context, accessToken string) ([]casdoor.Product, error) {
	if m.getProductsFn != nil {
		return m.getProductsFn(ctx, accessToken)
	}
	return nil, nil
}
func (m *mockCasdoorForPayment) GetUserInfo(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
	if m.getUserInfoFn != nil {
		return m.getUserInfoFn(ctx, accessToken)
	}
	return nil, nil
}
func (m *mockCasdoorForPayment) PlaceOrder(ctx context.Context, accessToken string, productName string) (*casdoor.Order, error) {
	if m.placeOrderFn != nil {
		return m.placeOrderFn(ctx, accessToken, productName)
	}
	return nil, nil
}
func (m *mockCasdoorForPayment) PayOrder(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error) {
	if m.payOrderFn != nil {
		return m.payOrderFn(ctx, accessToken, orderOwner, orderName, provider)
	}
	return nil, nil
}
func (m *mockCasdoorForPayment) GetOrder(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
	if m.getOrderFn != nil {
		return m.getOrderFn(ctx, accessToken, orderName)
	}
	return nil, nil
}
func (m *mockCasdoorForPayment) GetUserOrders(ctx context.Context, accessToken string, userName string) ([]casdoor.Order, error) {
	if m.getUserOrdersFn != nil {
		return m.getUserOrdersFn(ctx, accessToken, userName)
	}
	return nil, nil
}
func (m *mockCasdoorForPayment) CancelOrder(ctx context.Context, orderOwner string, orderName string) error {
	if m.cancelOrderFn != nil {
		return m.cancelOrderFn(ctx, orderOwner, orderName)
	}
	return nil
}

type mockEntitlementForPayment struct {
	grantResult *model.UserEntitlement
	grantErr    error
}

func (m *mockEntitlementForPayment) GetWallet(ctx context.Context, userID string) (*UserWalletInfo, error) {
	return nil, nil
}
func (m *mockEntitlementForPayment) ListEntitlements(ctx context.Context, userID string, limit, offset int) ([]EntitlementInfo, int64, error) {
	return nil, 0, nil
}
func (m *mockEntitlementForPayment) FreezeForTask(ctx context.Context, userID string, taskRef string, requiredSeconds int64) error {
	return nil
}
func (m *mockEntitlementForPayment) ConsumeTask(ctx context.Context, taskRef string) error  { return nil }
func (m *mockEntitlementForPayment) UnfreezeTask(ctx context.Context, taskRef string) error { return nil }
func (m *mockEntitlementForPayment) GrantEntitlement(ctx context.Context, userID string, productName string, orderID int64) (*model.UserEntitlement, error) {
	return m.grantResult, m.grantErr
}
func (m *mockEntitlementForPayment) GetBillingHistory(ctx context.Context, userID string, limit, offset int) ([]BillingHistoryEntry, int64, error) {
	return nil, 0, nil
}

// --- Helpers ---

func newTestPaymentService(
	casdoorClient *mockCasdoorForPayment,
	authService *mockAuthForPayment,
	billingRepo *mockBillingRepo,
	entitlementSvc *mockEntitlementForPayment,
) PaymentService {
	return NewPaymentService(casdoorClient, authService, billingRepo, entitlementSvc)
}

// --- GetProducts ---

func TestGetProducts_Success(t *testing.T) {
	client := &mockCasdoorForPayment{
		getProductsFn: func(ctx context.Context, accessToken string) ([]casdoor.Product, error) {
			return []casdoor.Product{
				{Name: "basic-pack", DisplayName: "Basic", Price: 10, Currency: "CNY", State: casdoor.ProductStatePublished},
				{Name: "premium-pack", DisplayName: "Premium", Price: 50, Currency: "CNY", State: casdoor.ProductStatePublished},
				{Name: "draft", DisplayName: "Draft", State: "Draft"},
			}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		productMappings: []model.ProductEntitlementMapping{
			{CasdoorProductName: "basic-pack", QuotaSeconds: 3600, EntitlementType: model.EntitlementTypeTopUp, PeriodMonths: 0},
		},
	}
	ent := &mockEntitlementForPayment{}
	svc := newTestPaymentService(client, auth, repo, ent)

	products, err := svc.GetProducts(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}

	if len(products) != 2 {
		t.Fatalf("expected 2 published products, got %d", len(products))
	}

	// basic-pack should have entitlement info
	if products[0].QuotaSeconds != 3600 {
		t.Errorf("basic-pack QuotaSeconds = %d, want 3600", products[0].QuotaSeconds)
	}
	if products[0].EntitlementType != "TOP_UP" {
		t.Errorf("basic-pack EntitlementType = %q, want %q", products[0].EntitlementType, "TOP_UP")
	}

	// premium-pack has no mapping
	if products[1].QuotaSeconds != 0 {
		t.Errorf("premium-pack QuotaSeconds = %d, want 0", products[1].QuotaSeconds)
	}
}

func TestGetProducts_TokenError(t *testing.T) {
	client := &mockCasdoorForPayment{}
	auth := &mockAuthForPayment{err: fmt.Errorf("no session")}
	repo := &mockBillingRepo{}
	ent := &mockEntitlementForPayment{}
	svc := newTestPaymentService(client, auth, repo, ent)

	_, err := svc.GetProducts(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error when token fails")
	}
	if !strings.Contains(err.Error(), "Casdoor token") {
		t.Errorf("error should mention Casdoor token, got: %v", err)
	}
}

func TestGetProducts_MappingError_StillReturnsProducts(t *testing.T) {
	client := &mockCasdoorForPayment{
		getProductsFn: func(ctx context.Context, accessToken string) ([]casdoor.Product, error) {
			return []casdoor.Product{
				{Name: "pack", State: casdoor.ProductStatePublished},
			}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{productMappingsErr: fmt.Errorf("db error")}
	ent := &mockEntitlementForPayment{}
	svc := newTestPaymentService(client, auth, repo, ent)

	products, err := svc.GetProducts(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetProducts should succeed even if mappings fail: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("expected 1 product, got %d", len(products))
	}
}

// --- CreateOrder ---

func TestCreateOrder_Success(t *testing.T) {
	client := &mockCasdoorForPayment{
		placeOrderFn: func(ctx context.Context, accessToken string, productName string) (*casdoor.Order, error) {
			return &casdoor.Order{
				Owner: "org", Name: "order-123", Price: 10, Currency: "CNY",
			}, nil
		},
		payOrderFn: func(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error) {
			return &casdoor.Payment{
				PayUrl: "https://pay.example.com/123", Price: 10, Currency: "CNY",
			}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{}
	ent := &mockEntitlementForPayment{}
	svc := newTestPaymentService(client, auth, repo, ent)

	result, err := svc.CreateOrder(context.Background(), "user-1", &CreateOrderInput{
		ProductName: "basic-pack",
		Provider:    "alipay",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if result.OrderID != "order-123" {
		t.Errorf("OrderID = %q, want %q", result.OrderID, "order-123")
	}
	if result.PaymentURL != "https://pay.example.com/123" {
		t.Errorf("PaymentURL = %q, want pay URL", result.PaymentURL)
	}
}

func TestCreateOrder_InvalidInput(t *testing.T) {
	svc := newTestPaymentService(&mockCasdoorForPayment{}, &mockAuthForPayment{}, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.CreateOrder(context.Background(), "user-1", &CreateOrderInput{Provider: "alipay"})
	if err == nil {
		t.Fatal("expected error for empty product name")
	}

	_, err = svc.CreateOrder(context.Background(), "user-1", &CreateOrderInput{ProductName: "pack"})
	if err == nil {
		t.Fatal("expected error for empty provider")
	}
}

func TestCreateOrder_ExistingOrder(t *testing.T) {
	client := &mockCasdoorForPayment{
		placeOrderFn: func(ctx context.Context, accessToken string, productName string) (*casdoor.Order, error) {
			return &casdoor.Order{Owner: "org", Name: "order-existing", Price: 20, Currency: "CNY"}, nil
		},
		payOrderFn: func(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error) {
			return &casdoor.Payment{PayUrl: "https://pay.example.com/existing", Price: 20, Currency: "CNY"}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{
			ID: 1, CasdoorOrderName: "order-existing", Status: model.OrderStatusCancelled,
		},
	}
	ent := &mockEntitlementForPayment{}
	svc := newTestPaymentService(client, auth, repo, ent)

	result, err := svc.CreateOrder(context.Background(), "user-1", &CreateOrderInput{
		ProductName: "pack", Provider: "alipay",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if result.OrderID != "order-existing" {
		t.Errorf("OrderID = %q, want %q", result.OrderID, "order-existing")
	}
}

// --- CancelOrder ---

func TestCancelOrder_Success(t *testing.T) {
	cancelCalled := false
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{Owner: "org", Name: "order-1", State: casdoor.OrderStateCreated}, nil
		},
		cancelOrderFn: func(ctx context.Context, orderOwner string, orderName string) error {
			cancelCalled = true
			return nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{ID: 1, CasdoorOrderName: "order-1", Status: model.OrderStatusPending},
	}
	ent := &mockEntitlementForPayment{}
	svc := newTestPaymentService(client, auth, repo, ent)

	err := svc.CancelOrder(context.Background(), "user-1", "order-1")
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if !cancelCalled {
		t.Error("CancelOrder should call casdoor cancel")
	}
}

func TestCancelOrder_WrongState(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	err := svc.CancelOrder(context.Background(), "user-1", "order-1")
	if err == nil {
		t.Fatal("expected error for non-Created order")
	}
	if !strings.Contains(err.Error(), "待支付") {
		t.Errorf("error should mention pending state, got: %v", err)
	}
}

// --- PayOrder ---

func TestPayOrder_Success(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{Owner: "org", Name: "order-1", State: casdoor.OrderStateCreated}, nil
		},
		payOrderFn: func(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error) {
			return &casdoor.Payment{PayUrl: "https://pay.example.com/1", Price: 10, Currency: "CNY"}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	result, err := svc.PayOrder(context.Background(), "user-1", "order-1", "alipay")
	if err != nil {
		t.Fatalf("PayOrder: %v", err)
	}
	if result.PaymentURL != "https://pay.example.com/1" {
		t.Errorf("PaymentURL = %q, want pay URL", result.PaymentURL)
	}
}

func TestPayOrder_EmptyProvider(t *testing.T) {
	svc := newTestPaymentService(&mockCasdoorForPayment{}, &mockAuthForPayment{}, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.PayOrder(context.Background(), "user-1", "order-1", "")
	if err == nil {
		t.Fatal("expected error for empty provider")
	}
}

func TestPayOrder_WrongState(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStateCanceled}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.PayOrder(context.Background(), "user-1", "order-1", "alipay")
	if err == nil {
		t.Fatal("expected error for non-Created order")
	}
}

// --- HandlePaymentCallback ---

func TestHandlePaymentCallback_Success(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{
			ID: 1, UserID: "user-1", CasdoorProductName: "pack", Status: model.OrderStatusPending,
		},
	}
	ent := &mockEntitlementForPayment{
		grantResult: &model.UserEntitlement{ID: 10, TotalSeconds: 3600},
	}
	svc := newTestPaymentService(client, auth, repo, ent)

	err := svc.HandlePaymentCallback(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("HandlePaymentCallback: %v", err)
	}
}

func TestHandlePaymentCallback_OrderNotFound(t *testing.T) {
	repo := &mockBillingRepo{} // no order
	svc := newTestPaymentService(&mockCasdoorForPayment{}, &mockAuthForPayment{}, repo, &mockEntitlementForPayment{})

	err := svc.HandlePaymentCallback(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing order")
	}
	if !strings.Contains(err.Error(), "订单不存在") {
		t.Errorf("error should mention order not found, got: %v", err)
	}
}

func TestHandlePaymentCallback_AlreadyPaid(t *testing.T) {
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{
			ID: 1, UserID: "user-1", Status: model.OrderStatusPaid,
		},
	}
	svc := newTestPaymentService(&mockCasdoorForPayment{}, &mockAuthForPayment{}, repo, &mockEntitlementForPayment{})

	err := svc.HandlePaymentCallback(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("already-paid order should be idempotent: %v", err)
	}
}

// --- SyncOrder ---

func TestSyncOrder_PaidAndGrant(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid, Name: "order-1"}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{
			ID: 1, UserID: "user-1", CasdoorProductName: "pack", Status: model.OrderStatusPending,
		},
	}
	ent := &mockEntitlementForPayment{
		grantResult: &model.UserEntitlement{ID: 10, TotalSeconds: 3600},
	}
	svc := newTestPaymentService(client, auth, repo, ent)

	result, err := svc.SyncOrder(context.Background(), "user-1", "order-1")
	if err != nil {
		t.Fatalf("SyncOrder: %v", err)
	}
	if result.OrderStatus != casdoor.OrderStatePaid {
		t.Errorf("OrderStatus = %q, want %q", result.OrderStatus, casdoor.OrderStatePaid)
	}
	if result.QuotaSeconds != 3600 {
		t.Errorf("QuotaSeconds = %d, want 3600", result.QuotaSeconds)
	}
	if !strings.Contains(result.Message, "成功") {
		t.Errorf("Message should mention success, got: %q", result.Message)
	}
}

func TestSyncOrder_AlreadyProcessed(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	grantedSeconds := int64(3600)
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{
			ID: 1, UserID: "user-1", Status: model.OrderStatusPaid, GrantedSeconds: &grantedSeconds,
		},
	}
	ent := &mockEntitlementForPayment{}
	svc := newTestPaymentService(client, auth, repo, ent)

	result, err := svc.SyncOrder(context.Background(), "user-1", "order-1")
	if err != nil {
		t.Fatalf("SyncOrder: %v", err)
	}
	if !strings.Contains(result.Message, "已处理") {
		t.Errorf("Message should mention already processed, got: %q", result.Message)
	}
	if result.QuotaSeconds != 3600 {
		t.Errorf("QuotaSeconds = %d, want 3600", result.QuotaSeconds)
	}
}

func TestSyncOrder_NotPaid(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStateCreated}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	result, err := svc.SyncOrder(context.Background(), "user-1", "order-1")
	if err != nil {
		t.Fatalf("SyncOrder: %v", err)
	}
	if result.OrderStatus != casdoor.OrderStateCreated {
		t.Errorf("OrderStatus = %q, want %q", result.OrderStatus, casdoor.OrderStateCreated)
	}
}

// --- GetOrders ---

// --- P1: CreateOrder nil req ---

func TestCreateOrder_NilReq(t *testing.T) {
	svc := newTestPaymentService(&mockCasdoorForPayment{}, &mockAuthForPayment{}, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.CreateOrder(context.Background(), "user-1", nil)
	if err == nil {
		t.Fatal("expected error for nil req")
	}
	if !strings.Contains(err.Error(), "不能为空") {
		t.Errorf("error should mention empty, got: %v", err)
	}
}

// --- P1: CreateOrder error paths ---

func TestCreateOrder_TokenError(t *testing.T) {
	auth := &mockAuthForPayment{err: fmt.Errorf("no session")}
	svc := newTestPaymentService(&mockCasdoorForPayment{}, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.CreateOrder(context.Background(), "user-1", &CreateOrderInput{ProductName: "pack", Provider: "alipay"})
	if err == nil {
		t.Fatal("expected error when token fails")
	}
	if !strings.Contains(err.Error(), "Casdoor token") {
		t.Errorf("error should mention token, got: %v", err)
	}
}

func TestCreateOrder_PlaceOrderError(t *testing.T) {
	client := &mockCasdoorForPayment{
		placeOrderFn: func(ctx context.Context, accessToken string, productName string) (*casdoor.Order, error) {
			return nil, fmt.Errorf("casdoor error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.CreateOrder(context.Background(), "user-1", &CreateOrderInput{ProductName: "pack", Provider: "alipay"})
	if err == nil {
		t.Fatal("expected error when PlaceOrder fails")
	}
	if !strings.Contains(err.Error(), "创建 Casdoor 订单") {
		t.Errorf("error should mention order creation, got: %v", err)
	}
}

func TestCreateOrder_PayOrderError(t *testing.T) {
	client := &mockCasdoorForPayment{
		placeOrderFn: func(ctx context.Context, accessToken string, productName string) (*casdoor.Order, error) {
			return &casdoor.Order{Owner: "org", Name: "order-1", Price: 10, Currency: "CNY"}, nil
		},
		payOrderFn: func(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error) {
			return nil, fmt.Errorf("payment gateway error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.CreateOrder(context.Background(), "user-1", &CreateOrderInput{ProductName: "pack", Provider: "alipay"})
	if err == nil {
		t.Fatal("expected error when PayOrder fails")
	}
	if !strings.Contains(err.Error(), "发起支付") {
		t.Errorf("error should mention payment, got: %v", err)
	}
}

func TestCreateOrder_Success_FieldAssertions(t *testing.T) {
	client := &mockCasdoorForPayment{
		placeOrderFn: func(ctx context.Context, accessToken string, productName string) (*casdoor.Order, error) {
			return &casdoor.Order{Owner: "org", Name: "order-123", Price: 29.9, Currency: "CNY"}, nil
		},
		payOrderFn: func(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error) {
			return &casdoor.Payment{PayUrl: "https://pay.example.com/123", Price: 29.9, Currency: "CNY"}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	result, err := svc.CreateOrder(context.Background(), "user-1", &CreateOrderInput{ProductName: "basic-pack", Provider: "alipay"})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if result.Amount != 29.9 {
		t.Errorf("Amount = %.2f, want 29.9", result.Amount)
	}
	if result.Currency != "CNY" {
		t.Errorf("Currency = %q, want %q", result.Currency, "CNY")
	}
}

// --- P1: CancelOrder error paths ---

func TestCancelOrder_TokenError(t *testing.T) {
	auth := &mockAuthForPayment{err: fmt.Errorf("no session")}
	svc := newTestPaymentService(&mockCasdoorForPayment{}, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	err := svc.CancelOrder(context.Background(), "user-1", "order-1")
	if err == nil {
		t.Fatal("expected error when token fails")
	}
}

func TestCancelOrder_GetOrderError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return nil, fmt.Errorf("casdoor error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	err := svc.CancelOrder(context.Background(), "user-1", "order-1")
	if err == nil {
		t.Fatal("expected error when GetOrder fails")
	}
	if !strings.Contains(err.Error(), "获取订单") {
		t.Errorf("error should mention order fetch, got: %v", err)
	}
}

func TestCancelOrder_CasdoorCancelError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{Owner: "org", Name: "order-1", State: casdoor.OrderStateCreated}, nil
		},
		cancelOrderFn: func(ctx context.Context, orderOwner string, orderName string) error {
			return fmt.Errorf("cancel failed")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	err := svc.CancelOrder(context.Background(), "user-1", "order-1")
	if err == nil {
		t.Fatal("expected error when Casdoor cancel fails")
	}
	if !strings.Contains(err.Error(), "取消订单") {
		t.Errorf("error should mention cancel, got: %v", err)
	}
}

// --- P1: PayOrder error paths ---

func TestPayOrder_TokenError(t *testing.T) {
	auth := &mockAuthForPayment{err: fmt.Errorf("no session")}
	svc := newTestPaymentService(&mockCasdoorForPayment{}, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.PayOrder(context.Background(), "user-1", "order-1", "alipay")
	if err == nil {
		t.Fatal("expected error when token fails")
	}
}

func TestPayOrder_GetOrderError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return nil, fmt.Errorf("casdoor error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.PayOrder(context.Background(), "user-1", "order-1", "alipay")
	if err == nil {
		t.Fatal("expected error when GetOrder fails")
	}
}

func TestPayOrder_CasdoorPayError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{Owner: "org", Name: "order-1", State: casdoor.OrderStateCreated}, nil
		},
		payOrderFn: func(ctx context.Context, accessToken string, orderOwner string, orderName string, provider string) (*casdoor.Payment, error) {
			return nil, fmt.Errorf("payment error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.PayOrder(context.Background(), "user-1", "order-1", "alipay")
	if err == nil {
		t.Fatal("expected error when Casdoor PayOrder fails")
	}
}

func TestPayOrder_AlreadyPaid(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.PayOrder(context.Background(), "user-1", "order-1", "alipay")
	if err == nil {
		t.Fatal("expected error for already-paid order")
	}
}

// --- P1: HandlePaymentCallback error paths ---

func TestHandlePaymentCallback_GetOrderError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return nil, fmt.Errorf("casdoor error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{ID: 1, UserID: "user-1", Status: model.OrderStatusPending},
	}
	svc := newTestPaymentService(client, auth, repo, &mockEntitlementForPayment{})

	err := svc.HandlePaymentCallback(context.Background(), "order-1")
	if err == nil {
		t.Fatal("expected error when GetOrder fails")
	}
}

func TestHandlePaymentCallback_OrderNotPaid(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStateCreated}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{ID: 1, UserID: "user-1", Status: model.OrderStatusPending},
	}
	svc := newTestPaymentService(client, auth, repo, &mockEntitlementForPayment{})

	err := svc.HandlePaymentCallback(context.Background(), "order-1")
	if err == nil {
		t.Fatal("expected error when order not paid")
	}
	if !strings.Contains(err.Error(), "未支付") {
		t.Errorf("error should mention unpaid, got: %v", err)
	}
}

func TestHandlePaymentCallback_GrantError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{ID: 1, UserID: "user-1", CasdoorProductName: "pack", Status: model.OrderStatusPending},
	}
	ent := &mockEntitlementForPayment{grantErr: fmt.Errorf("grant failed")}
	svc := newTestPaymentService(client, auth, repo, ent)

	err := svc.HandlePaymentCallback(context.Background(), "order-1")
	if err == nil {
		t.Fatal("expected error when GrantEntitlement fails")
	}
	if !strings.Contains(err.Error(), "权益") {
		t.Errorf("error should mention entitlement, got: %v", err)
	}
}

func TestHandlePaymentCallback_NilEntitlement(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{ID: 1, UserID: "user-1", CasdoorProductName: "pack", Status: model.OrderStatusPending},
	}
	ent := &mockEntitlementForPayment{grantResult: nil} // nil entitlement, no error
	svc := newTestPaymentService(client, auth, repo, ent)

	err := svc.HandlePaymentCallback(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("HandlePaymentCallback with nil entitlement should succeed: %v", err)
	}
}

func TestHandlePaymentCallback_TokenError(t *testing.T) {
	auth := &mockAuthForPayment{err: fmt.Errorf("no session")}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{ID: 1, UserID: "user-1", Status: model.OrderStatusPending},
	}
	svc := newTestPaymentService(&mockCasdoorForPayment{}, auth, repo, &mockEntitlementForPayment{})

	err := svc.HandlePaymentCallback(context.Background(), "order-1")
	if err == nil {
		t.Fatal("expected error when token fails")
	}
}

// --- P1: SyncOrder error paths ---

func TestSyncOrder_TokenError(t *testing.T) {
	auth := &mockAuthForPayment{err: fmt.Errorf("no session")}
	svc := newTestPaymentService(&mockCasdoorForPayment{}, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.SyncOrder(context.Background(), "user-1", "order-1")
	if err == nil {
		t.Fatal("expected error when token fails")
	}
}

func TestSyncOrder_GetOrderError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return nil, fmt.Errorf("casdoor error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.SyncOrder(context.Background(), "user-1", "order-1")
	if err == nil {
		t.Fatal("expected error when GetOrder fails")
	}
}

func TestSyncOrder_GrantError_NoPanic(t *testing.T) {
	// P0 regression: GrantEntitlement returns error, should not panic
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid, Name: "order-1"}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{ID: 1, UserID: "user-1", CasdoorProductName: "pack", Status: model.OrderStatusPending},
	}
	ent := &mockEntitlementForPayment{grantErr: fmt.Errorf("grant failed")}
	svc := newTestPaymentService(client, auth, repo, ent)

	result, err := svc.SyncOrder(context.Background(), "user-1", "order-1")
	if err != nil {
		t.Fatalf("SyncOrder should not return error on grant failure: %v", err)
	}
	if !strings.Contains(result.Message, "失败") {
		t.Errorf("Message should mention failure, got: %q", result.Message)
	}
}

func TestSyncOrder_NilEntitlement_NoPanic(t *testing.T) {
	// P0 regression: GrantEntitlement returns nil, nil — should not panic
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid, Name: "order-1"}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		orderByCasdoorName: &model.UserOrder{ID: 1, UserID: "user-1", CasdoorProductName: "pack", Status: model.OrderStatusPending},
	}
	ent := &mockEntitlementForPayment{grantResult: nil, grantErr: nil} // nil, nil
	svc := newTestPaymentService(client, auth, repo, ent)

	result, err := svc.SyncOrder(context.Background(), "user-1", "order-1")
	if err != nil {
		t.Fatalf("SyncOrder should succeed: %v", err)
	}
	if result.QuotaSeconds != 0 {
		t.Errorf("QuotaSeconds should be 0 for nil entitlement, got %d", result.QuotaSeconds)
	}
}

func TestSyncOrder_LocalOrderCreateError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getOrderFn: func(ctx context.Context, accessToken string, orderName string) (*casdoor.Order, error) {
			return &casdoor.Order{State: casdoor.OrderStatePaid, Name: "order-new", Products: []string{"pack"}}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{createOrderErr: fmt.Errorf("db error")}
	svc := newTestPaymentService(client, auth, repo, &mockEntitlementForPayment{})

	_, err := svc.SyncOrder(context.Background(), "user-1", "order-new")
	if err == nil {
		t.Fatal("expected error when CreateOrder fails")
	}
	if !strings.Contains(err.Error(), "创建本地订单") {
		t.Errorf("error should mention local order creation, got: %v", err)
	}
}

// --- P1: GetOrders error paths ---

func TestGetOrders_TokenError(t *testing.T) {
	auth := &mockAuthForPayment{err: fmt.Errorf("no session")}
	svc := newTestPaymentService(&mockCasdoorForPayment{}, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, _, err := svc.GetOrders(context.Background(), "user-1", 10, 0)
	if err == nil {
		t.Fatal("expected error when token fails")
	}
}

func TestGetOrders_UserInfoError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getUserInfoFn: func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
			return nil, fmt.Errorf("server error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, _, err := svc.GetOrders(context.Background(), "user-1", 10, 0)
	if err == nil {
		t.Fatal("expected error when GetUserInfo fails")
	}
}

func TestGetOrders_UserOrdersError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getUserInfoFn: func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
			return &casdoor.UserInfo{Name: "testuser"}, nil
		},
		getUserOrdersFn: func(ctx context.Context, accessToken string, userName string) ([]casdoor.Order, error) {
			return nil, fmt.Errorf("server error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, _, err := svc.GetOrders(context.Background(), "user-1", 10, 0)
	if err == nil {
		t.Fatal("expected error when GetUserOrders fails")
	}
}

func TestGetOrders_FieldAssertions(t *testing.T) {
	ts := time.Now().Format(time.RFC3339)
	client := &mockCasdoorForPayment{
		getUserInfoFn: func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
			return &casdoor.UserInfo{Name: "testuser"}, nil
		},
		getUserOrdersFn: func(ctx context.Context, accessToken string, userName string) ([]casdoor.Order, error) {
			return []casdoor.Order{
				{
					Name: "order-1", Price: 99.9, Currency: "USD",
					State: casdoor.OrderStateCreated, CreatedTime: ts,
					Products: []string{"pack"}, ProductInfos: []casdoor.ProductInfo{{DisplayName: "Test Pack"}},
				},
			}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	orders, _, err := svc.GetOrders(context.Background(), "user-1", 10, 0)
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(orders))
	}
	if orders[0].OrderID != "order-1" {
		t.Errorf("OrderID = %q, want %q", orders[0].OrderID, "order-1")
	}
	if orders[0].Price != 99.9 {
		t.Errorf("Price = %.2f, want 99.9", orders[0].Price)
	}
	if orders[0].Currency != "USD" {
		t.Errorf("Currency = %q, want %q", orders[0].Currency, "USD")
	}
	if orders[0].Status != casdoor.OrderStateCreated {
		t.Errorf("Status = %q, want %q", orders[0].Status, casdoor.OrderStateCreated)
	}
}

// --- P1: GetProducts error path ---

func TestGetProducts_CasdoorError(t *testing.T) {
	client := &mockCasdoorForPayment{
		getProductsFn: func(ctx context.Context, accessToken string) ([]casdoor.Product, error) {
			return nil, fmt.Errorf("casdoor server error")
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	_, err := svc.GetProducts(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error when Casdoor GetProducts fails")
	}
	if !strings.Contains(err.Error(), "产品列表") {
		t.Errorf("error should mention products, got: %v", err)
	}
}

func TestGetProducts_FieldAssertions(t *testing.T) {
	client := &mockCasdoorForPayment{
		getProductsFn: func(ctx context.Context, accessToken string) ([]casdoor.Product, error) {
			return []casdoor.Product{
				{Name: "basic-pack", DisplayName: "Basic Pack", Price: 29.9, Currency: "CNY",
					State: casdoor.ProductStatePublished, Image: "img.png",
					Description: "desc", IsRecharge: true, Providers: []string{"alipay"}},
			}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	repo := &mockBillingRepo{
		productMappings: []model.ProductEntitlementMapping{
			{CasdoorProductName: "basic-pack", QuotaSeconds: 7200, EntitlementType: model.EntitlementTypeTopUp, PeriodMonths: 1},
		},
	}
	svc := newTestPaymentService(client, auth, repo, &mockEntitlementForPayment{})

	products, err := svc.GetProducts(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("expected 1 product, got %d", len(products))
	}
	p := products[0]
	if p.Name != "basic-pack" {
		t.Errorf("Name = %q, want %q", p.Name, "basic-pack")
	}
	if p.DisplayName != "Basic Pack" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "Basic Pack")
	}
	if p.Price != 29.9 {
		t.Errorf("Price = %.2f, want 29.9", p.Price)
	}
	if p.Currency != "CNY" {
		t.Errorf("Currency = %q, want %q", p.Currency, "CNY")
	}
	if p.Image != "img.png" {
		t.Errorf("Image = %q, want %q", p.Image, "img.png")
	}
	if p.Description != "desc" {
		t.Errorf("Description = %q, want %q", p.Description, "desc")
	}
	if !p.IsRecharge {
		t.Error("IsRecharge should be true")
	}
	if len(p.Providers) != 1 || p.Providers[0] != "alipay" {
		t.Errorf("Providers = %v, want [alipay]", p.Providers)
	}
	if p.QuotaSeconds != 7200 {
		t.Errorf("QuotaSeconds = %d, want 7200", p.QuotaSeconds)
	}
	if p.PeriodMonths != 1 {
		t.Errorf("PeriodMonths = %d, want 1", p.PeriodMonths)
	}
}

func TestGetOrders_Success(t *testing.T) {
	now := time.Now().Format(time.RFC3339)
	client := &mockCasdoorForPayment{
		getUserInfoFn: func(ctx context.Context, accessToken string) (*casdoor.UserInfo, error) {
			return &casdoor.UserInfo{Name: "testuser"}, nil
		},
		getUserOrdersFn: func(ctx context.Context, accessToken string, userName string) ([]casdoor.Order, error) {
			return []casdoor.Order{
				{
					Name: "order-1", Price: 10, Currency: "CNY",
					State: casdoor.OrderStateCreated, CreatedTime: now,
					Products: []string{"pack"}, ProductInfos: []casdoor.ProductInfo{{DisplayName: "Test Pack"}},
				},
				{Name: "order-2", State: casdoor.OrderStateCanceled, CreatedTime: now}, // should be filtered
			}, nil
		},
	}
	auth := &mockAuthForPayment{token: "valid-token"}
	svc := newTestPaymentService(client, auth, &mockBillingRepo{}, &mockEntitlementForPayment{})

	orders, total, err := svc.GetOrders(context.Background(), "user-1", 10, 0)
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 non-canceled order, got %d", len(orders))
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if orders[0].DisplayName != "Test Pack" {
		t.Errorf("DisplayName = %q, want %q", orders[0].DisplayName, "Test Pack")
	}
}
