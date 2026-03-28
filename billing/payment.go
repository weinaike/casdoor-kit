package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/weinaike/casdoor-kit"
	"github.com/weinaike/casdoor-kit/authz"
	"github.com/weinaike/casdoor-kit/authz/casdoor"
	"github.com/weinaike/casdoor-kit/billing/model"
	"github.com/weinaike/casdoor-kit/billing/repo"
)

// PaymentService handles payment and order operations.
type PaymentService interface {
	GetProducts(ctx context.Context, userID string) ([]ProductWithEntitlement, error)
	CreateOrder(ctx context.Context, userID string, req *CreateOrderInput) (*PaymentResult, error)
	GetOrders(ctx context.Context, userID string, limit, offset int) ([]OrderHistory, int64, error)
	CancelOrder(ctx context.Context, userID string, orderName string) error
	PayOrder(ctx context.Context, userID string, orderName string, provider string) (*PaymentResult, error)
	HandlePaymentCallback(ctx context.Context, orderName string) error
	SyncOrder(ctx context.Context, userID string, orderName string) (*OrderSyncResult, error)
}

// CreateOrderInput is the input for creating an order.
type CreateOrderInput struct {
	ProductName string `json:"product_name"`
	Provider    string `json:"provider"`
}

// ProductWithEntitlement combines Casdoor product with local entitlement info.
type ProductWithEntitlement struct {
	Name            string   `json:"name"`
	DisplayName     string   `json:"display_name"`
	Description     string   `json:"description"`
	Image           string   `json:"image"`
	Price           float64  `json:"price"`
	Currency        string   `json:"currency"`
	IsRecharge      bool     `json:"is_recharge"`
	Providers       []string `json:"providers"`
	QuotaSeconds    int64    `json:"quota_seconds"`
	EntitlementType string   `json:"entitlement_type"`
	PeriodMonths    int      `json:"period_months"`
	State           string   `json:"state"`
}

// PaymentResult is the result of a payment initiation.
type PaymentResult struct {
	OrderID    string  `json:"order_id"`
	PaymentURL string  `json:"payment_url"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
}

// OrderHistory is an order history entry.
type OrderHistory struct {
	OrderID     string     `json:"order_id"`
	ProductName string     `json:"product_name"`
	DisplayName string     `json:"display_name"`
	Price       float64    `json:"price"`
	Currency    string     `json:"currency"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
}

// OrderSyncResult is the result of an order sync.
type OrderSyncResult struct {
	OrderStatus  string `json:"order_status"`
	QuotaSeconds int64  `json:"quota_seconds"`
	Message      string `json:"message"`
}

type paymentService struct {
	casdoorClient      casdoor.ClientInterface
	authService        authz.AuthService
	entitlementRepo    repo.BillingRepository
	entitlementService EntitlementService
}

// NewPaymentService creates a payment service.
func NewPaymentService(
	casdoorClient casdoor.ClientInterface,
	authService authz.AuthService,
	entitlementRepo repo.BillingRepository,
	entitlementService EntitlementService,
) PaymentService {
	return &paymentService{
		casdoorClient:      casdoorClient,
		authService:        authService,
		entitlementRepo:    entitlementRepo,
		entitlementService: entitlementService,
	}
}

func (s *paymentService) GetProducts(ctx context.Context, userID string) ([]ProductWithEntitlement, error) {
	accessToken, err := s.authService.GetCasdoorToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取 Casdoor token 失败: %w", err)
	}

	casdoorProducts, err := s.casdoorClient.GetProducts(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("获取 Casdoor 产品列表失败: %w", err)
	}

	mappings, err := s.entitlementRepo.ListActiveProductMappings(ctx)
	if err != nil {
		gokit.GetLogger().Warn("获取产品权益映射失败", "error", err)
		mappings = nil
	}

	mappingMap := make(map[string]model.ProductEntitlementMapping)
	for _, m := range mappings {
		mappingMap[m.CasdoorProductName] = m
	}

	products := make([]ProductWithEntitlement, 0, len(casdoorProducts))
	for _, p := range casdoorProducts {
		if p.State != casdoor.ProductStatePublished {
			continue
		}

		product := ProductWithEntitlement{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Description: p.Description,
			Image:       p.Image,
			Price:       p.Price,
			Currency:    p.Currency,
			IsRecharge:   p.IsRecharge,
			Providers:   p.Providers,
			State:       p.State,
		}

		if mapping, ok := mappingMap[p.Name]; ok {
			product.QuotaSeconds = mapping.QuotaSeconds
			product.EntitlementType = string(mapping.EntitlementType)
			product.PeriodMonths = mapping.PeriodMonths
		}

		products = append(products, product)
	}

	return products, nil
}

func (s *paymentService) CreateOrder(ctx context.Context, userID string, req *CreateOrderInput) (*PaymentResult, error) {
	if req == nil {
		return nil, errors.New("请求参数不能为空")
	}
	if req.ProductName == "" {
		return nil, errors.New("产品名称不能为空")
	}
	if req.Provider == "" {
		return nil, errors.New("支付方式不能为空")
	}

	accessToken, err := s.authService.GetCasdoorToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取 Casdoor token 失败: %w", err)
	}

	casdoorOrder, err := s.casdoorClient.PlaceOrder(ctx, accessToken, req.ProductName)
	if err != nil {
		return nil, fmt.Errorf("创建 Casdoor 订单失败: %w", err)
	}

	localOrder, err := s.entitlementRepo.GetOrderByCasdoorOrderName(ctx, casdoorOrder.Name)
	if err != nil {
		return nil, fmt.Errorf("查询本地订单失败: %w", err)
	}

	if localOrder == nil {
		localOrder = &model.UserOrder{
			UserID:             userID,
			CasdoorOrderName:   casdoorOrder.Name,
			CasdoorProductName: req.ProductName,
			Price:              casdoorOrder.Price,
			Currency:           casdoorOrder.Currency,
			Status:             model.OrderStatusPending,
		}
		if err := s.entitlementRepo.CreateOrder(ctx, localOrder); err != nil {
			return nil, fmt.Errorf("创建本地订单失败: %w", err)
		}
	} else {
		localOrder.Price = casdoorOrder.Price
		localOrder.Currency = casdoorOrder.Currency
		if localOrder.Status == model.OrderStatusCancelled {
			localOrder.Status = model.OrderStatusPending
		}
		if err := s.entitlementRepo.UpdateOrder(ctx, localOrder); err != nil {
			gokit.GetLogger().Error("更新本地订单失败", "error", err)
		}
	}

	payment, err := s.casdoorClient.PayOrder(ctx, accessToken, casdoorOrder.Owner, casdoorOrder.Name, req.Provider)
	if err != nil {
		return nil, fmt.Errorf("发起支付失败: %w", err)
	}

	return &PaymentResult{
		OrderID:    casdoorOrder.Name,
		PaymentURL: payment.PayUrl,
		Amount:     payment.Price,
		Currency:   payment.Currency,
	}, nil
}

func (s *paymentService) GetOrders(ctx context.Context, userID string, limit, offset int) ([]OrderHistory, int64, error) {
	accessToken, err := s.authService.GetCasdoorToken(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("获取 Casdoor token 失败: %w", err)
	}

	userInfo, err := s.casdoorClient.GetUserInfo(ctx, accessToken)
	if err != nil {
		return nil, 0, fmt.Errorf("获取用户信息失败: %w", err)
	}

	casdoorOrders, err := s.casdoorClient.GetUserOrders(ctx, accessToken, userInfo.Name)
	if err != nil {
		return nil, 0, fmt.Errorf("获取 Casdoor 订单列表失败: %w", err)
	}

	orders := make([]OrderHistory, 0, len(casdoorOrders))
	for _, o := range casdoorOrders {
		if o.State == casdoor.OrderStateCanceled {
			continue
		}

		productName := ""
		displayName := ""
		if len(o.Products) > 0 {
			productName = o.Products[0]
		}
		if len(o.ProductInfos) > 0 {
			displayName = o.ProductInfos[0].DisplayName
		}
		if displayName == "" {
			displayName = productName
		}

		var paidAt *time.Time
		if o.State == casdoor.OrderStatePaid {
			if t, err := time.Parse(time.RFC3339, o.UpdateTime); err == nil {
				paidAt = &t
			}
		}

		createdAt, _ := time.Parse(time.RFC3339, o.CreatedTime)

		orders = append(orders, OrderHistory{
			OrderID:     o.Name,
			ProductName: productName,
			DisplayName: displayName,
			Price:       o.Price,
			Currency:    o.Currency,
			Status:      o.State,
			CreatedAt:   createdAt,
			PaidAt:      paidAt,
		})
	}

	return orders, int64(len(orders)), nil
}

func (s *paymentService) CancelOrder(ctx context.Context, userID string, orderName string) error {
	accessToken, err := s.authService.GetCasdoorToken(ctx, userID)
	if err != nil {
		return fmt.Errorf("获取 Casdoor token 失败: %w", err)
	}

	casdoorOrder, err := s.casdoorClient.GetOrder(ctx, accessToken, orderName)
	if err != nil {
		return fmt.Errorf("获取订单失败: %w", err)
	}

	if casdoorOrder.State != casdoor.OrderStateCreated {
		return fmt.Errorf("只能取消待支付的订单，当前状态: %s", casdoorOrder.State)
	}

	if err := s.casdoorClient.CancelOrder(ctx, casdoorOrder.Owner, orderName); err != nil {
		return fmt.Errorf("取消订单失败: %w", err)
	}

	localOrder, err := s.entitlementRepo.GetOrderByCasdoorOrderName(ctx, orderName)
	if err != nil {
		gokit.GetLogger().Warn("获取本地订单失败", "error", err)
		return nil
	}

	if localOrder != nil {
		localOrder.Status = model.OrderStatusCancelled
		if err := s.entitlementRepo.UpdateOrder(ctx, localOrder); err != nil {
			gokit.GetLogger().Warn("更新本地订单状态失败", "error", err)
		}
	}

	gokit.GetLogger().Info("订单取消成功", "order_name", orderName, "user_id", userID)
	return nil
}

func (s *paymentService) PayOrder(ctx context.Context, userID string, orderName string, provider string) (*PaymentResult, error) {
	if provider == "" {
		return nil, errors.New("支付方式不能为空")
	}

	accessToken, err := s.authService.GetCasdoorToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取 Casdoor token 失败: %w", err)
	}

	casdoorOrder, err := s.casdoorClient.GetOrder(ctx, accessToken, orderName)
	if err != nil {
		return nil, fmt.Errorf("获取订单失败: %w", err)
	}

	if casdoorOrder.State != casdoor.OrderStateCreated {
		return nil, fmt.Errorf("只能支付待支付的订单，当前状态: %s", casdoorOrder.State)
	}

	payment, err := s.casdoorClient.PayOrder(ctx, accessToken, casdoorOrder.Owner, orderName, provider)
	if err != nil {
		return nil, fmt.Errorf("发起支付失败: %w", err)
	}

	return &PaymentResult{
		OrderID:    orderName,
		PaymentURL: payment.PayUrl,
		Amount:     payment.Price,
		Currency:   payment.Currency,
	}, nil
}

func (s *paymentService) HandlePaymentCallback(ctx context.Context, orderName string) error {
	order, err := s.entitlementRepo.GetOrderByCasdoorOrderName(ctx, orderName)
	if err != nil {
		return fmt.Errorf("获取本地订单失败: %w", err)
	}
	if order == nil {
		return fmt.Errorf("订单不存在: %s", orderName)
	}

	if order.Status == model.OrderStatusPaid {
		return nil
	}

	accessToken, err := s.authService.GetCasdoorToken(ctx, order.UserID)
	if err != nil {
		return fmt.Errorf("获取 Casdoor token 失败: %w", err)
	}

	casdoorOrder, err := s.casdoorClient.GetOrder(ctx, accessToken, orderName)
	if err != nil {
		return fmt.Errorf("获取 Casdoor 订单失败: %w", err)
	}

	if casdoorOrder.State != casdoor.OrderStatePaid {
		return fmt.Errorf("订单未支付: %s", casdoorOrder.State)
	}

	entitlement, err := s.entitlementService.GrantEntitlement(ctx, order.UserID, order.CasdoorProductName, order.ID)
	if err != nil {
		return fmt.Errorf("发放权益失败: %w", err)
	}

	now := time.Now()
	order.Status = model.OrderStatusPaid
	order.PaidAt = &now
	if entitlement != nil {
		order.GrantedSeconds = &entitlement.TotalSeconds
		order.EntitlementID = &entitlement.ID
	}
	if err := s.entitlementRepo.UpdateOrder(ctx, order); err != nil {
		gokit.GetLogger().Error("更新本地订单状态失败", "error", err)
	}

	logMsg := "支付回调处理成功"
	logArgs := []interface{}{
		"order_name", orderName,
		"user_id", order.UserID,
	}
	if entitlement != nil {
		logArgs = append(logArgs, "granted_seconds", entitlement.TotalSeconds)
	}
	gokit.GetLogger().Info(logMsg, logArgs...)

	return nil
}

func (s *paymentService) SyncOrder(ctx context.Context, userID string, orderName string) (*OrderSyncResult, error) {
	accessToken, err := s.authService.GetCasdoorToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取 Casdoor token 失败: %w", err)
	}

	casdoorOrder, err := s.casdoorClient.GetOrder(ctx, accessToken, orderName)
	if err != nil {
		return nil, fmt.Errorf("获取 Casdoor 订单失败: %w", err)
	}

	result := &OrderSyncResult{OrderStatus: casdoorOrder.State}

	if casdoorOrder.State != casdoor.OrderStatePaid {
		result.Message = fmt.Sprintf("订单状态: %s", casdoorOrder.State)
		return result, nil
	}

	localOrder, err := s.entitlementRepo.GetOrderByCasdoorOrderName(ctx, orderName)
	if err != nil {
		return nil, fmt.Errorf("查询本地订单失败: %w", err)
	}

	if localOrder == nil {
		productName := ""
		if len(casdoorOrder.Products) > 0 {
			productName = casdoorOrder.Products[0]
		}
		localOrder = &model.UserOrder{
			UserID:             userID,
			CasdoorOrderName:   casdoorOrder.Name,
			CasdoorProductName: productName,
			Price:              casdoorOrder.Price,
			Currency:           casdoorOrder.Currency,
			Status:             model.OrderStatusPending,
		}
		if err := s.entitlementRepo.CreateOrder(ctx, localOrder); err != nil {
			return nil, fmt.Errorf("创建本地订单失败: %w", err)
		}
	}

	if localOrder.Status == model.OrderStatusPaid {
		result.Message = "订单已处理"
		if localOrder.GrantedSeconds != nil {
			result.QuotaSeconds = *localOrder.GrantedSeconds
		}
		return result, nil
	}

	entitlement, err := s.entitlementService.GrantEntitlement(ctx, userID, localOrder.CasdoorProductName, localOrder.ID)
	if err != nil {
		gokit.GetLogger().Error("发放权益失败", "order_name", orderName, "error", err)
		result.Message = "权益发放失败，请稍后重试"
		return result, nil
	}

	now := time.Now()
	localOrder.Status = model.OrderStatusPaid
	localOrder.PaidAt = &now
	if entitlement != nil {
		localOrder.GrantedSeconds = &entitlement.TotalSeconds
		localOrder.EntitlementID = &entitlement.ID
	}
	if err := s.entitlementRepo.UpdateOrder(ctx, localOrder); err != nil {
		gokit.GetLogger().Error("更新本地订单状态失败", "error", err)
	}

	if entitlement != nil {
		result.QuotaSeconds = entitlement.TotalSeconds
	}
	result.Message = "支付成功，权益已发放"

	gokit.GetLogger().Info("订单同步成功，权益已发放",
		"order_name", orderName,
		"user_id", userID)

	return result, nil
}
