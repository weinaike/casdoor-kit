package billing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/weinaike/casdoor-kit/billing/model"
	"github.com/weinaike/casdoor-kit/billing/repo"
	"github.com/weinaike/casdoor-kit/config"
)

// ---------------------------------------------------------------------------
// mockBillingRepo - hand-written mock implementing repo.BillingRepository
// ---------------------------------------------------------------------------

type mockBillingRepo struct {
	// GetWalletByUserID
	wallet    *model.UserWallet
	walletErr error

	// GetActiveEntitlementsByUserID
	activeEntitlements    []model.UserEntitlement
	activeEntitlementsErr error

	// ListEntitlementsByUserID
	listEntitlements    []model.UserEntitlement
	listEntitlementsTotal int64
	listEntitlementsErr error

	// ListActiveProductMappings
	productMappings    []model.ProductEntitlementMapping
	productMappingsErr error

	// GetProductMappingByProductName
	productMapping    *model.ProductEntitlementMapping
	productMappingErr error

	// Freeze
	freezeResult    *model.TaskBilling
	freezeErr       error

	// Consume
	consumeErr error

	// Unfreeze
	unfreezeErr error

	// GrantEntitlement (repo-level)
	grantResult    *model.UserEntitlement
	grantErr       error

	// ListBillingTransactionsByUserID
	transactions    []model.BillingTransactionLog
	transactionsTotal int64
	transactionsErr error

	// Order
	orderByCasdoorName    *model.UserOrder
	orderByCasdoorNameErr error
	createOrderErr        error
	updateOrderErr        error
}

// --- Implement all 26 methods of repo.BillingRepository ---

func (m *mockBillingRepo) GetProductMappingByProductName(_ context.Context, _ string) (*model.ProductEntitlementMapping, error) {
	return m.productMapping, m.productMappingErr
}

func (m *mockBillingRepo) ListActiveProductMappings(_ context.Context) ([]model.ProductEntitlementMapping, error) {
	return m.productMappings, m.productMappingsErr
}

func (m *mockBillingRepo) GetWalletByUserID(_ context.Context, _ string) (*model.UserWallet, error) {
	return m.wallet, m.walletErr
}

func (m *mockBillingRepo) CreateWallet(_ context.Context, _ string) (*model.UserWallet, error) {
	return nil, nil
}

func (m *mockBillingRepo) UpdateWalletWithVersion(_ context.Context, _ *model.UserWallet) error {
	return nil
}

func (m *mockBillingRepo) GetActiveEntitlementsByUserID(_ context.Context, _ string) ([]model.UserEntitlement, error) {
	return m.activeEntitlements, m.activeEntitlementsErr
}

func (m *mockBillingRepo) GetActiveEntitlementsCountByUserID(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockBillingRepo) ListEntitlementsByUserID(_ context.Context, _ string, _, _ int) ([]model.UserEntitlement, int64, error) {
	return m.listEntitlements, m.listEntitlementsTotal, m.listEntitlementsErr
}

func (m *mockBillingRepo) GetEntitlementByID(_ context.Context, _ int64) (*model.UserEntitlement, error) {
	return nil, nil
}

func (m *mockBillingRepo) CreateEntitlement(_ context.Context, _ *model.UserEntitlement) error {
	return nil
}

func (m *mockBillingRepo) UpdateEntitlement(_ context.Context, _ *model.UserEntitlement) error {
	return nil
}

func (m *mockBillingRepo) CreateBillingTransaction(_ context.Context, _ *model.BillingTransactionLog) error {
	return nil
}

func (m *mockBillingRepo) ListBillingTransactionsByUserID(_ context.Context, _ string, _, _ int) ([]model.BillingTransactionLog, int64, error) {
	return m.transactions, m.transactionsTotal, m.transactionsErr
}

func (m *mockBillingRepo) CreateOrder(_ context.Context, _ *model.UserOrder) error {
	return m.createOrderErr
}

func (m *mockBillingRepo) GetOrderByCasdoorOrderName(_ context.Context, _ string) (*model.UserOrder, error) {
	return m.orderByCasdoorName, m.orderByCasdoorNameErr
}

func (m *mockBillingRepo) GetOrderByID(_ context.Context, _ int64) (*model.UserOrder, error) {
	return nil, nil
}

func (m *mockBillingRepo) UpdateOrder(_ context.Context, _ *model.UserOrder) error {
	return m.updateOrderErr
}

func (m *mockBillingRepo) ListOrdersByUserID(_ context.Context, _ string, _, _ int) ([]model.UserOrder, int64, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) CreateTaskBilling(_ context.Context, _ *model.TaskBilling) error {
	return nil
}

func (m *mockBillingRepo) GetTaskBillingByTaskRef(_ context.Context, _ string) (*model.TaskBilling, error) {
	return nil, nil
}

func (m *mockBillingRepo) UpdateTaskBilling(_ context.Context, _ *model.TaskBilling) error {
	return nil
}

func (m *mockBillingRepo) Freeze(_ context.Context, _ string, _ string, _ int64) (*model.TaskBilling, error) {
	return m.freezeResult, m.freezeErr
}

func (m *mockBillingRepo) Consume(_ context.Context, _ string) error {
	return m.consumeErr
}

func (m *mockBillingRepo) Unfreeze(_ context.Context, _ string) error {
	return m.unfreezeErr
}

func (m *mockBillingRepo) GrantEntitlement(_ context.Context, _ string, _ *model.ProductEntitlementMapping, _ int64) (*model.UserEntitlement, error) {
	return m.grantResult, m.grantErr
}

// --- Helper to create the service under test ---

func newTestService(m *mockBillingRepo, cfg *config.EntitlementConfig) EntitlementService {
	return NewEntitlementService(m, cfg)
}

// compile-time check
var _ repo.BillingRepository = (*mockBillingRepo)(nil)

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// ---- GetWallet ----

func TestGetWallet_WalletExists(t *testing.T) {
	mock := &mockBillingRepo{
		wallet: &model.UserWallet{
			TotalAvailable: 3600,
			TotalFrozen:    600,
		},
		activeEntitlements: []model.UserEntitlement{{ID: 1}, {ID: 2}},
	}

	svc := newTestService(mock, nil)
	info, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.TotalSeconds != 3600 {
		t.Errorf("TotalSeconds = %d, want 3600", info.TotalSeconds)
	}
	if info.FrozenSeconds != 600 {
		t.Errorf("FrozenSeconds = %d, want 600", info.FrozenSeconds)
	}
	if info.AvailableSeconds != 3000 {
		t.Errorf("AvailableSeconds = %d, want 3000", info.AvailableSeconds)
	}
	if info.EntitlementsCount != 2 {
		t.Errorf("EntitlementsCount = %d, want 2", info.EntitlementsCount)
	}
}

func TestGetWallet_Nil(t *testing.T) {
	mock := &mockBillingRepo{
		wallet: nil,
	}

	svc := newTestService(mock, nil)
	info, err := svc.GetWallet(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil UserWalletInfo, got nil")
	}
	if info.TotalSeconds != 0 || info.FrozenSeconds != 0 || info.AvailableSeconds != 0 || info.EntitlementsCount != 0 {
		t.Errorf("expected zero-valued UserWalletInfo, got %+v", info)
	}
}

func TestGetWallet_RepoError(t *testing.T) {
	mock := &mockBillingRepo{
		walletErr: errors.New("db down"),
	}

	svc := newTestService(mock, nil)
	_, err := svc.GetWallet(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "获取用户钱包失败") {
		t.Errorf("error message should contain '获取用户钱包失败', got: %v", err)
	}
}

// ---- FreezeForTask ----

func TestFreezeForTask_Success(t *testing.T) {
	mock := &mockBillingRepo{
		freezeResult: &model.TaskBilling{
			FrozenDetails: model.FrozenDetails{
				{EntitlementID: 1, Seconds: 100},
			},
		},
	}

	svc := newTestService(mock, nil)
	err := svc.FreezeForTask(context.Background(), "user-1", "task-abc", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFreezeForTask_ZeroSeconds(t *testing.T) {
	mock := &mockBillingRepo{}
	svc := newTestService(mock, nil)

	err := svc.FreezeForTask(context.Background(), "user-1", "task-abc", 0)
	if err == nil {
		t.Fatal("expected error for zero seconds, got nil")
	}
	if err.Error() != "冻结时长必须大于0" {
		t.Errorf("error = %q, want %q", err.Error(), "冻结时长必须大于0")
	}
}

func TestFreezeForTask_InsufficientBalance(t *testing.T) {
	mock := &mockBillingRepo{
		freezeErr: repo.ErrInsufficientBalance,
	}

	svc := newTestService(mock, nil)
	err := svc.FreezeForTask(context.Background(), "user-1", "task-abc", 500)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "余额不足") {
		t.Errorf("error should contain '余额不足', got: %v", err)
	}
}

func TestFreezeForTask_RepoError(t *testing.T) {
	mock := &mockBillingRepo{
		freezeErr: errors.New("connection refused"),
	}

	svc := newTestService(mock, nil)
	err := svc.FreezeForTask(context.Background(), "user-1", "task-abc", 100)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "冻结权益失败") {
		t.Errorf("error should contain '冻结权益失败', got: %v", err)
	}
}

// ---- ConsumeTask ----

func TestConsumeTask_Success(t *testing.T) {
	mock := &mockBillingRepo{}
	svc := newTestService(mock, nil)

	err := svc.ConsumeTask(context.Background(), "task-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConsumeTask_Error(t *testing.T) {
	mock := &mockBillingRepo{
		consumeErr: errors.New("not found"),
	}
	svc := newTestService(mock, nil)

	err := svc.ConsumeTask(context.Background(), "task-abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "核销权益失败") {
		t.Errorf("error should contain '核销权益失败', got: %v", err)
	}
}

// ---- UnfreezeTask ----

func TestUnfreezeTask_Success(t *testing.T) {
	mock := &mockBillingRepo{}
	svc := newTestService(mock, nil)

	err := svc.UnfreezeTask(context.Background(), "task-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---- GrantEntitlement ----

func TestGrantEntitlement_ConfigMapping(t *testing.T) {
	mock := &mockBillingRepo{
		grantResult: &model.UserEntitlement{ID: 10, TotalSeconds: 3600},
	}

	cfg := &config.EntitlementConfig{
		ProductMappings: map[string]config.ProductMapping{
			"test-product": {
				QuotaSeconds:    3600,
				EntitlementType: "TOP_UP",
				PeriodMonths:    0,
				Description:     "Test product",
			},
		},
	}

	svc := newTestService(mock, cfg)
	ent, err := svc.GrantEntitlement(context.Background(), "user-1", "test-product", 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.ID != 10 {
		t.Errorf("entitlement ID = %d, want 10", ent.ID)
	}
	// When config mapping is found, repo.GetProductMappingByProductName should NOT be called.
	// mock.productMapping is nil (default), so if it were called we'd get a "产品权益映射不存在" error.
}

func TestGrantEntitlement_ConfigMapping_Subscription(t *testing.T) {
	mock := &mockBillingRepo{
		grantResult: &model.UserEntitlement{ID: 20, TotalSeconds: 7200},
	}

	cfg := &config.EntitlementConfig{
		ProductMappings: map[string]config.ProductMapping{
			"sub-product": {
				QuotaSeconds:    7200,
				EntitlementType: "SUBSCRIPTION",
				PeriodMonths:    1,
				Description:     "Monthly subscription",
			},
		},
	}

	svc := newTestService(mock, cfg)
	ent, err := svc.GrantEntitlement(context.Background(), "user-1", "sub-product", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.ID != 20 {
		t.Errorf("entitlement ID = %d, want 20", ent.ID)
	}
}

func TestGrantEntitlement_DbFallback(t *testing.T) {
	mock := &mockBillingRepo{
		productMapping: &model.ProductEntitlementMapping{
			CasdoorProductName: "db-product",
			QuotaSeconds:       1800,
			EntitlementType:    model.EntitlementTypeGift,
			PeriodMonths:       0,
			Description:        "DB product",
			IsActive:           true,
		},
		grantResult: &model.UserEntitlement{ID: 30, TotalSeconds: 1800},
	}

	// Empty config - no product mappings defined.
	cfg := &config.EntitlementConfig{ProductMappings: map[string]config.ProductMapping{}}

	svc := newTestService(mock, cfg)
	ent, err := svc.GrantEntitlement(context.Background(), "user-1", "db-product", 101)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.ID != 30 {
		t.Errorf("entitlement ID = %d, want 30", ent.ID)
	}
}

func TestGrantEntitlement_NoMapping(t *testing.T) {
	mock := &mockBillingRepo{
		// productMapping is nil by default, simulating "not found".
	}

	cfg := &config.EntitlementConfig{ProductMappings: map[string]config.ProductMapping{}}

	svc := newTestService(mock, cfg)
	_, err := svc.GrantEntitlement(context.Background(), "user-1", "nonexistent", 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "产品权益映射不存在") {
		t.Errorf("error should contain '产品权益映射不存在', got: %v", err)
	}
}

// ---- ListEntitlements ----

func TestListEntitlements(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	mock := &mockBillingRepo{
		listEntitlements: []model.UserEntitlement{
			{
				ID:                 1,
				SourceType:         model.EntitlementTypeTopUp,
				TotalSeconds:       3600,
				UsedSeconds:        600,
				FrozenSeconds:      200,
				ValidFrom:          now,
				ValidUntil:         nil,
				Status:             model.EntitlementStatusActive,
				CasdoorProductName: "basic-pack",
				CreatedAt:          now,
			},
		},
		listEntitlementsTotal: 1,
		productMappings: []model.ProductEntitlementMapping{
			{
				CasdoorProductName: "basic-pack",
				Description:        "基础套餐",
			},
		},
	}

	svc := newTestService(mock, nil)
	items, total, err := svc.ListEntitlements(context.Background(), "user-1", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	item := items[0]
	if item.ID != 1 {
		t.Errorf("ID = %d, want 1", item.ID)
	}
	if item.SourceType != model.EntitlementTypeTopUp {
		t.Errorf("SourceType = %q, want %q", item.SourceType, model.EntitlementTypeTopUp)
	}
	if item.TotalSeconds != 3600 {
		t.Errorf("TotalSeconds = %d, want 3600", item.TotalSeconds)
	}
	if item.UsedSeconds != 600 {
		t.Errorf("UsedSeconds = %d, want 600", item.UsedSeconds)
	}
	// AvailableSeconds = TotalSeconds - UsedSeconds - FrozenSeconds = 3600 - 600 - 200 = 2800
	if item.AvailableSeconds != 2800 {
		t.Errorf("AvailableSeconds = %d, want 2800", item.AvailableSeconds)
	}
	// DisplayName should come from product mapping description.
	if item.DisplayName != "基础套餐" {
		t.Errorf("DisplayName = %q, want %q", item.DisplayName, "基础套餐")
	}
	if item.ValidUntil != nil {
		t.Errorf("ValidUntil should be nil for permanent entitlement, got %v", item.ValidUntil)
	}
	if item.ValidFrom != now.Format("2006-01-02T15:04:05Z07:00") {
		t.Errorf("ValidFrom = %q, want %q", item.ValidFrom, now.Format("2006-01-02T15:04:05Z07:00"))
	}
}

// ---- GetBillingHistory ----

func TestGetBillingHistory(t *testing.T) {
	createdAt := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)
	taskRef := "task-uuid-123"

	mock := &mockBillingRepo{
		transactions: []model.BillingTransactionLog{
			{
				ID:            1,
				ActionType:    model.BillingActionFreeze,
				AmountSeconds: 300,
				BalanceAfter:  3300,
				Description:   "任务冻结: task-uuid-123",
				TaskRef:       &taskRef,
				CreatedAt:     createdAt,
			},
		},
		transactionsTotal: 1,
	}

	svc := newTestService(mock, nil)
	entries, total, err := svc.GetBillingHistory(context.Background(), "user-1", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	entry := entries[0]
	if entry.ID != 1 {
		t.Errorf("ID = %d, want 1", entry.ID)
	}
	if entry.ActionType != model.BillingActionFreeze {
		t.Errorf("ActionType = %q, want %q", entry.ActionType, model.BillingActionFreeze)
	}
	if entry.AmountSeconds != 300 {
		t.Errorf("AmountSeconds = %d, want 300", entry.AmountSeconds)
	}
	if entry.BalanceAfter != 3300 {
		t.Errorf("BalanceAfter = %d, want 3300", entry.BalanceAfter)
	}
	// TaskRef from model should map to JobUUID in DTO.
	if entry.JobUUID == nil || *entry.JobUUID != "task-uuid-123" {
		t.Errorf("JobUUID = %v, want %q", entry.JobUUID, "task-uuid-123")
	}
	expectedDate := createdAt.Format("2006-01-02T15:04:05Z07:00")
	if entry.CreatedAt != expectedDate {
		t.Errorf("CreatedAt = %q, want %q", entry.CreatedAt, expectedDate)
	}
}
