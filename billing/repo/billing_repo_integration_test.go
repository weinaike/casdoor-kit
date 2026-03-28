//go:build integration

package repo

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/weinaike/casdoor-kit/billing/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// uuid generates a random UUID v4 string for use as taskRef.
func uuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var testDB *gorm.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:32530/fengyn?sslmode=disable"
	}

	var err error
	testDB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect database: %v\n", err)
		os.Exit(1)
	}

	if err := model.AutoMigrate(testDB); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}

// cleanAll truncates all billing tables in the current test.
func cleanAll(t *testing.T) {
	t.Helper()
	tables := []string{"billing_transaction_log", "job_billing", "user_entitlement", "user_wallet", "user_order", "product_entitlement_mapping"}
	for _, tbl := range tables {
		if err := testDB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", tbl)).Error; err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
}

// seedWallet creates a wallet with the given balance.
func seedWallet(t *testing.T, userID string, totalAvailable int64) *model.UserWallet {
	t.Helper()
	w := &model.UserWallet{UserID: userID, TotalAvailable: totalAvailable, Version: 1}
	if err := testDB.Create(w).Error; err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	return w
}

// seedEntitlement creates an active entitlement.
func seedEntitlement(t *testing.T, userID string, totalSeconds int64, sourceType model.EntitlementType) *model.UserEntitlement {
	t.Helper()
	e := &model.UserEntitlement{
		UserID:       userID,
		SourceType:   sourceType,
		TotalSeconds: totalSeconds,
		ValidFrom:    time.Now(),
		Status:       model.EntitlementStatusActive,
	}
	if err := testDB.Create(e).Error; err != nil {
		t.Fatalf("seed entitlement: %v", err)
	}
	return e
}

// seedMapping creates a product mapping.
func seedMapping(t *testing.T, productName string, quotaSeconds int64, entType model.EntitlementType, periodMonths int) *model.ProductEntitlementMapping {
	t.Helper()
	m := &model.ProductEntitlementMapping{
		CasdoorProductName: productName,
		QuotaSeconds:       quotaSeconds,
		EntitlementType:    entType,
		PeriodMonths:       periodMonths,
		IsActive:           true,
	}
	if err := testDB.Create(m).Error; err != nil {
		t.Fatalf("seed mapping: %v", err)
	}
	return m
}

// --- Tests ---

func TestFreezeConsumeCycle(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-fc"

	seedWallet(t, userID, 3600)
	seedEntitlement(t, userID, 3600, model.EntitlementTypeTopUp)
	taskRef := uuid()

	// Freeze
	tb, err := repo.Freeze(ctx, userID, taskRef, 1200)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if tb.Status != model.TaskBillingStatusProcessing {
		t.Errorf("task status = %q, want PROCESSING", tb.Status)
	}
	if tb.BilledSeconds != 1200 {
		t.Errorf("billed_seconds = %d, want 1200", tb.BilledSeconds)
	}
	if len(tb.FrozenDetails) != 1 {
		t.Fatalf("frozen details len = %d, want 1", len(tb.FrozenDetails))
	}
	if tb.FrozenDetails[0].Seconds != 1200 {
		t.Errorf("frozen seconds = %d, want 1200", tb.FrozenDetails[0].Seconds)
	}

	// Verify wallet frozen
	wallet, _ := repo.GetWalletByUserID(ctx, userID)
	if wallet.TotalFrozen != 1200 {
		t.Errorf("total_frozen = %d, want 1200", wallet.TotalFrozen)
	}

	// Verify entitlement frozen
	ents, _ := repo.GetActiveEntitlementsByUserID(ctx, userID)
	if len(ents) == 0 || ents[0].FrozenSeconds != 1200 {
		t.Errorf("entitlement frozen_seconds = %d, want 1200", ents[0].FrozenSeconds)
	}

	// Consume
	if err := repo.Consume(ctx, taskRef); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	// Verify after consume
	wallet, _ = repo.GetWalletByUserID(ctx, userID)
	if wallet.TotalAvailable != 2400 {
		t.Errorf("total_available = %d, want 2400", wallet.TotalAvailable)
	}
	if wallet.TotalFrozen != 0 {
		t.Errorf("total_frozen = %d, want 0", wallet.TotalFrozen)
	}

	// Verify entitlement consumed
	ents, _ = repo.GetActiveEntitlementsByUserID(ctx, userID)
	if len(ents) == 0 || ents[0].UsedSeconds != 1200 {
		t.Errorf("entitlement used_seconds = %d, want 1200", ents[0].UsedSeconds)
	}
	if ents[0].FrozenSeconds != 0 {
		t.Errorf("entitlement frozen_seconds = %d, want 0", ents[0].FrozenSeconds)
	}

	// Verify billing transactions
	txs, _, _ := repo.ListBillingTransactionsByUserID(ctx, userID, 10, 0)
	if len(txs) != 2 {
		t.Fatalf("transactions count = %d, want 2", len(txs))
	}
	if txs[1].ActionType != model.BillingActionFreeze {
		t.Errorf("tx[1] action = %q, want FREEZE", txs[1].ActionType)
	}
	if txs[0].ActionType != model.BillingActionConsume {
		t.Errorf("tx[0] action = %q, want CONSUME", txs[0].ActionType)
	}

	// Verify task billing status
	tb2, _ := repo.GetTaskBillingByTaskRef(ctx, taskRef)
	if tb2.Status != model.TaskBillingStatusSuccess {
		t.Errorf("task status = %q, want SUCCESS", tb2.Status)
	}
}

func TestFreezeUnfreezeCycle(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-fu"

	seedWallet(t, userID, 3600)
	seedEntitlement(t, userID, 3600, model.EntitlementTypeTopUp)
	taskRef := uuid()

	// Freeze
	_, err := repo.Freeze(ctx, userID, taskRef, 2000)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}

	// Verify wallet frozen
	wallet, _ := repo.GetWalletByUserID(ctx, userID)
	if wallet.TotalFrozen != 2000 {
		t.Errorf("total_frozen = %d, want 2000", wallet.TotalFrozen)
	}

	// Unfreeze (task failed)
	if err := repo.Unfreeze(ctx, taskRef); err != nil {
		t.Fatalf("Unfreeze: %v", err)
	}

	// Verify balance restored
	wallet, _ = repo.GetWalletByUserID(ctx, userID)
	if wallet.TotalAvailable != 3600 {
		t.Errorf("total_available = %d, want 3600", wallet.TotalAvailable)
	}
	if wallet.TotalFrozen != 0 {
		t.Errorf("total_frozen = %d, want 0", wallet.TotalFrozen)
	}

	// Verify entitlement restored
	ents, _ := repo.GetActiveEntitlementsByUserID(ctx, userID)
	if len(ents) == 0 || ents[0].FrozenSeconds != 0 {
		t.Errorf("entitlement frozen_seconds = %d, want 0", ents[0].FrozenSeconds)
	}
	if ents[0].UsedSeconds != 0 {
		t.Errorf("entitlement used_seconds = %d, want 0", ents[0].UsedSeconds)
	}

	// Verify task billing status
	tb, _ := repo.GetTaskBillingByTaskRef(ctx, taskRef)
	if tb.Status != model.TaskBillingStatusFailed {
		t.Errorf("task status = %q, want FAILED", tb.Status)
	}
}

func TestFreeze_InsufficientBalance(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-nobal"

	seedWallet(t, userID, 600)
	seedEntitlement(t, userID, 600, model.EntitlementTypeTopUp)

	_, err := repo.Freeze(ctx, userID, uuid(), 1200)
	if err == nil {
		t.Fatal("expected error for insufficient balance")
	}
	if err != ErrInsufficientBalance {
		t.Errorf("error = %v, want ErrInsufficientBalance", err)
	}
}

func TestFreeze_Idempotent(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-idem"

	seedWallet(t, userID, 3600)
	seedEntitlement(t, userID, 3600, model.EntitlementTypeTopUp)
	taskRef := uuid()

	tb1, err := repo.Freeze(ctx, userID, taskRef, 500)
	if err != nil {
		t.Fatalf("first Freeze: %v", err)
	}

	tb2, err := repo.Freeze(ctx, userID, taskRef, 500)
	if err != nil {
		t.Fatalf("second Freeze: %v", err)
	}

	if tb1.ID != tb2.ID {
		t.Errorf("idempotent freeze returned different records: %d vs %d", tb1.ID, tb2.ID)
	}

	// Verify only frozen once
	wallet, _ := repo.GetWalletByUserID(ctx, userID)
	if wallet.TotalFrozen != 500 {
		t.Errorf("total_frozen = %d, want 500 (frozen only once)", wallet.TotalFrozen)
	}
}

func TestConsume_ExhaustsEntitlement(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-exhaust"

	seedWallet(t, userID, 100)
	ent := seedEntitlement(t, userID, 100, model.EntitlementTypeTopUp)
	taskRef := uuid()

	_, err := repo.Freeze(ctx, userID, taskRef, 100)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}

	if err := repo.Consume(ctx, taskRef); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	// Entitlement should be EXHAUSTED
	updated, _ := repo.GetEntitlementByID(ctx, ent.ID)
	if updated.Status != model.EntitlementStatusExhausted {
		t.Errorf("status = %q, want EXHAUSTED", updated.Status)
	}
	if updated.AvailableSeconds() != 0 {
		t.Errorf("available = %d, want 0", updated.AvailableSeconds())
	}

	// Should not be in active entitlements list
	active, _ := repo.GetActiveEntitlementsByUserID(ctx, userID)
	for _, a := range active {
		if a.ID == ent.ID {
			t.Error("exhausted entitlement should not appear in active list")
		}
	}
}

func TestFreeze_MultipleEntitlements(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-multi"

	seedWallet(t, userID, 2000)
	// Two entitlements: GIFT (priority 1) and TOP_UP (priority 3)
	seedEntitlement(t, userID, 500, model.EntitlementTypeGift)
	seedEntitlement(t, userID, 1500, model.EntitlementTypeTopUp)

	// Freeze 1200 seconds — should take 500 from GIFT + 700 from TOP_UP
	taskRef := uuid()
	tb, err := repo.Freeze(ctx, userID, taskRef, 1200)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if len(tb.FrozenDetails) != 2 {
		t.Fatalf("frozen details len = %d, want 2", len(tb.FrozenDetails))
	}

	// Verify amounts
	totalFrozen := int64(0)
	for _, d := range tb.FrozenDetails {
		totalFrozen += d.Seconds
	}
	if totalFrozen != 1200 {
		t.Errorf("total frozen = %d, want 1200", totalFrozen)
	}

	// Consume
	if err := repo.Consume(ctx, taskRef); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	// GIFT should be exhausted, TOP_UP should have remaining
	active, _ := repo.GetActiveEntitlementsByUserID(ctx, userID)
	if len(active) != 1 {
		t.Fatalf("active entitlements = %d, want 1", len(active))
	}
	if active[0].SourceType != model.EntitlementTypeTopUp {
		t.Errorf("remaining entitlement type = %q, want TOP_UP", active[0].SourceType)
	}
	if active[0].AvailableSeconds() != 800 {
		t.Errorf("remaining available = %d, want 800", active[0].AvailableSeconds())
	}
}

func TestGrantEntitlement_TopUp(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-grant"

	// User has no wallet yet
	mapping := seedMapping(t, "test-pack", 3600, model.EntitlementTypeTopUp, 0)

	ent, err := repo.GrantEntitlement(ctx, userID, mapping, 0)
	if err != nil {
		t.Fatalf("GrantEntitlement: %v", err)
	}

	if ent.UserID != userID {
		t.Errorf("user_id = %q, want %q", ent.UserID, userID)
	}
	if ent.TotalSeconds != 3600 {
		t.Errorf("total_seconds = %d, want 3600", ent.TotalSeconds)
	}
	if ent.Status != model.EntitlementStatusActive {
		t.Errorf("status = %q, want ACTIVE", ent.Status)
	}
	if ent.ValidUntil != nil {
		t.Error("TOP_UP should have nil valid_until")
	}

	// Wallet should be auto-created
	wallet, _ := repo.GetWalletByUserID(ctx, userID)
	if wallet == nil {
		t.Fatal("wallet should be auto-created")
	}
	if wallet.TotalAvailable != 3600 {
		t.Errorf("total_available = %d, want 3600", wallet.TotalAvailable)
	}

	// Verify billing transaction
	txs, total, _ := repo.ListBillingTransactionsByUserID(ctx, userID, 10, 0)
	if total != 1 {
		t.Errorf("transactions = %d, want 1", total)
	}
	if txs[0].ActionType != model.BillingActionGrant {
		t.Errorf("action = %q, want GRANT", txs[0].ActionType)
	}
}

func TestGrantEntitlement_Subscription(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-sub"

	seedWallet(t, userID, 0)
	mapping := seedMapping(t, "pro-monthly", 7200, model.EntitlementTypeSubscription, 1)

	ent, err := repo.GrantEntitlement(ctx, userID, mapping, 100)
	if err != nil {
		t.Fatalf("GrantEntitlement: %v", err)
	}

	if ent.ValidUntil == nil {
		t.Error("SUBSCRIPTION should have valid_until set")
	}
	if ent.OrderID == nil || *ent.OrderID != 100 {
		t.Errorf("order_id = %v, want 100", ent.OrderID)
	}

	wallet, _ := repo.GetWalletByUserID(ctx, userID)
	if wallet.TotalAvailable != 7200 {
		t.Errorf("total_available = %d, want 7200", wallet.TotalAvailable)
	}
}

func TestWalletVersionConflict(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-version"

	wallet := seedWallet(t, userID, 3600)

	// Simulate concurrent update by modifying the DB directly
	testDB.Exec("UPDATE user_wallet SET version = version + 1 WHERE id = ?", wallet.ID)

	// Now try to update with stale version
	wallet.TotalAvailable = 3000
	err := repo.UpdateWalletWithVersion(ctx, wallet)
	if err == nil {
		t.Error("expected version conflict error")
	}
}

func TestConsume_Idempotent(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-consume-idem"

	seedWallet(t, userID, 3600)
	seedEntitlement(t, userID, 3600, model.EntitlementTypeTopUp)
	taskRef := uuid()

	_, err := repo.Freeze(ctx, userID, taskRef, 500)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}

	// First consume
	if err := repo.Consume(ctx, taskRef); err != nil {
		t.Fatalf("first Consume: %v", err)
	}

	// Get balance after first consume
	wallet1, _ := repo.GetWalletByUserID(ctx, userID)

	// Second consume should be no-op (status is SUCCESS, not PROCESSING)
	if err := repo.Consume(ctx, taskRef); err != nil {
		t.Fatalf("second Consume: %v", err)
	}

	// Balance should not have changed
	wallet2, _ := repo.GetWalletByUserID(ctx, userID)
	if wallet2.TotalAvailable != wallet1.TotalAvailable {
		t.Errorf("balance changed after idempotent consume: %d -> %d", wallet1.TotalAvailable, wallet2.TotalAvailable)
	}
}

func TestUnfreeze_Idempotent(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-unfreeze-idem"

	seedWallet(t, userID, 3600)
	seedEntitlement(t, userID, 3600, model.EntitlementTypeTopUp)
	taskRef := uuid()

	_, err := repo.Freeze(ctx, userID, taskRef, 500)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}

	if err := repo.Unfreeze(ctx, taskRef); err != nil {
		t.Fatalf("first Unfreeze: %v", err)
	}

	wallet1, _ := repo.GetWalletByUserID(ctx, userID)

	// Second unfreeze should be no-op
	if err := repo.Unfreeze(ctx, taskRef); err != nil {
		t.Fatalf("second Unfreeze: %v", err)
	}

	wallet2, _ := repo.GetWalletByUserID(ctx, userID)
	if wallet2.TotalFrozen != wallet1.TotalFrozen {
		t.Errorf("frozen changed after idempotent unfreeze: %d -> %d", wallet1.TotalFrozen, wallet2.TotalFrozen)
	}
}

func TestFreezeConsume_MultipleTasksSequentially(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-sequential"

	seedWallet(t, userID, 10000)
	seedEntitlement(t, userID, 10000, model.EntitlementTypeTopUp)

	// Run 5 freeze→consume cycles sequentially
	for i := 0; i < 5; i++ {
		taskRef := uuid()
		_, err := repo.Freeze(ctx, userID, taskRef, 1000)
		if err != nil {
			t.Fatalf("Freeze %d: %v", i, err)
		}
		if err := repo.Consume(ctx, taskRef); err != nil {
			t.Fatalf("Consume %d: %v", i, err)
		}
	}

	// Final balance should be 10000 - 5*1000 = 5000
	wallet, _ := repo.GetWalletByUserID(ctx, userID)
	if wallet.TotalAvailable != 5000 {
		t.Errorf("total_available = %d, want 5000", wallet.TotalAvailable)
	}
	if wallet.TotalFrozen != 0 {
		t.Errorf("total_frozen = %d, want 0", wallet.TotalFrozen)
	}
}

func TestFreezeConsume_ConcurrentDifferentUsers(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)

	var wg sync.WaitGroup
	errors := make(chan error, 5)

	// Different users can safely run concurrently
	for i := 0; i < 5; i++ {
		userID := fmt.Sprintf("test-concurrent-user-%d", i)
		seedWallet(t, userID, 5000)
		seedEntitlement(t, userID, 5000, model.EntitlementTypeTopUp)

		wg.Add(1)
		go func(uid string) {
			defer wg.Done()
			taskRef := uuid()
			_, err := repo.Freeze(ctx, uid, taskRef, 1000)
			if err != nil {
				errors <- err
				return
			}
			if err := repo.Consume(ctx, taskRef); err != nil {
				errors <- err
			}
		}(userID)
	}
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent task error: %v", err)
	}

	// Each user should have 4000 remaining
	for i := 0; i < 5; i++ {
		userID := fmt.Sprintf("test-concurrent-user-%d", i)
		wallet, _ := repo.GetWalletByUserID(ctx, userID)
		if wallet.TotalAvailable != 4000 {
			t.Errorf("user %d: total_available = %d, want 4000", i, wallet.TotalAvailable)
		}
	}
}

func TestListOperations(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-list"

	seedWallet(t, userID, 5000)
	seedEntitlement(t, userID, 5000, model.EntitlementTypeTopUp)

	// Freeze and consume to create billing history
	repo.Freeze(ctx, userID, "aaaaaaaa-0000-4000-8000-000000000001", 100)
	repo.Consume(ctx, "aaaaaaaa-0000-4000-8000-000000000001")
	repo.Freeze(ctx, userID, "aaaaaaaa-0000-4000-8000-000000000002", 200)
	repo.Unfreeze(ctx, "aaaaaaaa-0000-4000-8000-000000000002")

	// List entitlements
	ents, entTotal, err := repo.ListEntitlementsByUserID(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("ListEntitlements: %v", err)
	}
	if entTotal != 1 {
		t.Errorf("entitlement total = %d, want 1", entTotal)
	}
	if len(ents) != 1 {
		t.Errorf("entitlements len = %d, want 1", len(ents))
	}

	// List billing transactions
	txs, txTotal, err := repo.ListBillingTransactionsByUserID(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("ListBillingTransactions: %v", err)
	}
	// Should have: GRANT(from seed? no), FREEZE(1), CONSUME(1), FREEZE(1), UNFREEZE(1) = 4
	if txTotal != 4 {
		t.Errorf("transaction total = %d, want 4", txTotal)
	}
	if len(txs) != 4 {
		t.Errorf("transactions len = %d, want 4", len(txs))
	}

	// Verify pagination
	txs2, _, _ := repo.ListBillingTransactionsByUserID(ctx, userID, 2, 0)
	if len(txs2) != 2 {
		t.Errorf("paginated transactions len = %d, want 2", len(txs2))
	}
}

func TestOrderCRUD(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-order"

	order := &model.UserOrder{
		UserID:             userID,
		CasdoorOrderName:   "order_test_001",
		CasdoorProductName: "basic-pack",
		Price:              9.9,
		Currency:           "CNY",
		Status:             model.OrderStatusPending,
	}
	if err := repo.CreateOrder(ctx, order); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.ID == 0 {
		t.Error("order ID should be set after creation")
	}

	// Get by casdoor order name
	got, err := repo.GetOrderByCasdoorOrderName(ctx, "order_test_001")
	if err != nil {
		t.Fatalf("GetOrderByCasdoorOrderName: %v", err)
	}
	if got.UserID != userID {
		t.Errorf("user_id = %q, want %q", got.UserID, userID)
	}

	// Get by ID
	got2, err := repo.GetOrderByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrderByID: %v", err)
	}
	if got2.CasdoorOrderName != "order_test_001" {
		t.Errorf("order_name = %q", got2.CasdoorOrderName)
	}

	// Update
	got.Status = model.OrderStatusPaid
	now := time.Now()
	got.PaidAt = &now
	if err := repo.UpdateOrder(ctx, got); err != nil {
		t.Fatalf("UpdateOrder: %v", err)
	}

	updated, _ := repo.GetOrderByID(ctx, order.ID)
	if updated.Status != model.OrderStatusPaid {
		t.Errorf("status = %q, want Paid", updated.Status)
	}

	// List orders
	orders, total, err := repo.ListOrdersByUserID(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("ListOrdersByUserID: %v", err)
	}
	if total != 1 {
		t.Errorf("orders total = %d, want 1", total)
	}
	if len(orders) != 1 {
		t.Errorf("orders len = %d, want 1", len(orders))
	}
}

func TestProductMappingCRUD(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)

	_ = seedMapping(t, "test-product", 5000, model.EntitlementTypeTopUp, 0)

	// Get by name
	got, err := repo.GetProductMappingByProductName(ctx, "test-product")
	if err != nil {
		t.Fatalf("GetProductMappingByProductName: %v", err)
	}
	if got.QuotaSeconds != 5000 {
		t.Errorf("quota = %d, want 5000", got.QuotaSeconds)
	}

	// Not found
	got, err = repo.GetProductMappingByProductName(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent product")
	}

	// List active
	seedMapping(t, "inactive-product", 1000, model.EntitlementTypeTopUp, 0)
	testDB.Exec("UPDATE product_entitlement_mapping SET is_active = false WHERE casdoor_product_name = 'inactive-product'")

	mappings, err := repo.ListActiveProductMappings(ctx)
	if err != nil {
		t.Fatalf("ListActiveProductMappings: %v", err)
	}
	if len(mappings) != 1 {
		t.Errorf("active mappings = %d, want 1", len(mappings))
	}
}

func TestWalletCRUD(t *testing.T) {
	cleanAll(t)
	ctx := context.Background()
	repo := NewBillingRepository(testDB)
	userID := "test-user-wallet"

	// Not found returns nil
	w, err := repo.GetWalletByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != nil {
		t.Error("expected nil for nonexistent wallet")
	}

	// Create
	w, err = repo.CreateWallet(ctx, userID)
	if err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}
	if w.UserID != userID {
		t.Errorf("user_id = %q", w.UserID)
	}
	if w.Version != 1 {
		t.Errorf("version = %d, want 1", w.Version)
	}

	// Get after create
	w2, _ := repo.GetWalletByUserID(ctx, userID)
	if w2.TotalAvailable != 0 {
		t.Errorf("total_available = %d, want 0", w2.TotalAvailable)
	}

	// Update with version
	w2.TotalAvailable = 5000
	if err := repo.UpdateWalletWithVersion(ctx, w2); err != nil {
		t.Fatalf("UpdateWalletWithVersion: %v", err)
	}
	w3, _ := repo.GetWalletByUserID(ctx, userID)
	if w3.TotalAvailable != 5000 {
		t.Errorf("total_available = %d, want 5000", w3.TotalAvailable)
	}
	if w3.Version != 2 {
		t.Errorf("version = %d, want 2", w3.Version)
	}
}
