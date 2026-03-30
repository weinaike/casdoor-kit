package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/weinaike/casdoor-kit/billing/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Sentinel errors for billing operations
var (
	ErrInsufficientBalance = errors.New("余额不足")
)

// BillingRepository is the billing data access interface.
type BillingRepository interface {
	GetProductMappingByProductName(ctx context.Context, productName string) (*model.ProductEntitlementMapping, error)
	ListActiveProductMappings(ctx context.Context) ([]model.ProductEntitlementMapping, error)
	GetWalletByUserID(ctx context.Context, userID string) (*model.UserWallet, error)
	CreateWallet(ctx context.Context, userID string) (*model.UserWallet, error)
	UpdateWalletWithVersion(ctx context.Context, wallet *model.UserWallet) error
	GetActiveEntitlementsByUserID(ctx context.Context, userID string) ([]model.UserEntitlement, error)
	GetActiveEntitlementsCountByUserID(ctx context.Context, userID string) (int, error)
	ListEntitlementsByUserID(ctx context.Context, userID string, limit, offset int) ([]model.UserEntitlement, int64, error)
	GetEntitlementByID(ctx context.Context, id int64) (*model.UserEntitlement, error)
	CreateEntitlement(ctx context.Context, entitlement *model.UserEntitlement) error
	UpdateEntitlement(ctx context.Context, entitlement *model.UserEntitlement) error
	CreateBillingTransaction(ctx context.Context, tx *model.BillingTransactionLog) error
	ListBillingTransactionsByUserID(ctx context.Context, userID string, limit, offset int) ([]model.BillingTransactionLog, int64, error)
	CreateOrder(ctx context.Context, order *model.UserOrder) error
	GetOrderByCasdoorOrderName(ctx context.Context, orderName string) (*model.UserOrder, error)
	GetOrderByID(ctx context.Context, id int64) (*model.UserOrder, error)
	UpdateOrder(ctx context.Context, order *model.UserOrder) error
	ListOrdersByUserID(ctx context.Context, userID string, limit, offset int) ([]model.UserOrder, int64, error)
	CreateTaskBilling(ctx context.Context, taskBilling *model.TaskBilling) error
	GetTaskBillingByTaskRef(ctx context.Context, taskRef string) (*model.TaskBilling, error)
	UpdateTaskBilling(ctx context.Context, taskBilling *model.TaskBilling) error
	Freeze(ctx context.Context, userID string, taskRef string, requiredSeconds int64) (*model.TaskBilling, error)
	Consume(ctx context.Context, taskRef string) error
	Unfreeze(ctx context.Context, taskRef string) error
	GrantEntitlement(ctx context.Context, userID string, mapping *model.ProductEntitlementMapping, orderID int64) (*model.UserEntitlement, error)
	// CanUserPurchaseProduct checks if a user can still purchase a product based on max_per_user
	CanUserPurchaseProduct(ctx context.Context, userID string, productName string) (bool, error)
	// ExpireEntitlements processes expired entitlements in batches and adjusts wallet balances
	ExpireEntitlements(ctx context.Context, batchSize int) (expired int64, err error)
	// ReconcileWallet recalculates wallet balance from active entitlements
	ReconcileWallet(ctx context.Context, userID string) (before, after int64, err error)
	// ListUsersWithWalletDiscrepancy finds users whose wallet doesn't match entitlements
	ListUsersWithWalletDiscrepancy(ctx context.Context, limit int) ([]string, error)
}

type billingRepo struct {
	db *gorm.DB
}

// NewBillingRepository creates a billing repository.
func NewBillingRepository(db *gorm.DB) BillingRepository {
	return &billingRepo{db: db}
}

func (r *billingRepo) GetProductMappingByProductName(ctx context.Context, productName string) (*model.ProductEntitlementMapping, error) {
	var mapping model.ProductEntitlementMapping
	if err := r.db.WithContext(ctx).
		Where("casdoor_product_name = ? AND is_active = ?", productName, true).
		First(&mapping).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("获取产品权益映射失败: %w", err)
	}
	return &mapping, nil
}

func (r *billingRepo) ListActiveProductMappings(ctx context.Context) ([]model.ProductEntitlementMapping, error) {
	var mappings []model.ProductEntitlementMapping
	if err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("created_at ASC").
		Find(&mappings).Error; err != nil {
		return nil, fmt.Errorf("获取产品权益映射列表失败: %w", err)
	}
	return mappings, nil
}

func (r *billingRepo) GetWalletByUserID(ctx context.Context, userID string) (*model.UserWallet, error) {
	var wallet model.UserWallet
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&wallet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("获取用户钱包失败: %w", err)
	}
	return &wallet, nil
}

func (r *billingRepo) CreateWallet(ctx context.Context, userID string) (*model.UserWallet, error) {
	wallet := &model.UserWallet{
		UserID:         userID,
		TotalAvailable: 0,
		TotalFrozen:    0,
		Version:        1,
	}
	if err := r.db.WithContext(ctx).Create(wallet).Error; err != nil {
		return nil, fmt.Errorf("创建用户钱包失败: %w", err)
	}
	return wallet, nil
}

func (r *billingRepo) UpdateWalletWithVersion(ctx context.Context, wallet *model.UserWallet) error {
	result := r.db.WithContext(ctx).
		Model(&model.UserWallet{}).
		Where("id = ? AND version = ?", wallet.ID, wallet.Version).
		Updates(map[string]interface{}{
			"total_available": wallet.TotalAvailable,
			"total_frozen":    wallet.TotalFrozen,
			"version":         wallet.Version + 1,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("更新用户钱包失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("钱包版本冲突，请重试")
	}
	wallet.Version++
	return nil
}

func (r *billingRepo) GetActiveEntitlementsByUserID(ctx context.Context, userID string) ([]model.UserEntitlement, error) {
	var entitlements []model.UserEntitlement
	query := `
		SELECT * FROM user_entitlement
		WHERE user_id = ?
			AND status = 'ACTIVE'
			AND (valid_until IS NULL OR valid_until > NOW())
			AND (total_seconds - used_seconds - frozen_seconds) > 0
		ORDER BY
			CASE WHEN valid_until IS NULL THEN 1 ELSE 0 END,
			valid_until ASC,
			CASE source_type
				WHEN 'GIFT' THEN 1
				WHEN 'SUBSCRIPTION' THEN 2
				WHEN 'TOP_UP' THEN 3
				WHEN 'TRIAL' THEN 4
			END
	`
	if err := r.db.WithContext(ctx).Raw(query, userID).Scan(&entitlements).Error; err != nil {
		return nil, fmt.Errorf("获取用户权益包失败: %w", err)
	}
	return entitlements, nil
}

func (r *billingRepo) GetActiveEntitlementsCountByUserID(ctx context.Context, userID string) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.UserEntitlement{}).
		Where("user_id = ? AND status = ?", userID, model.EntitlementStatusActive).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("统计用户权益包数量失败: %w", err)
	}
	return int(count), nil
}

func (r *billingRepo) ListEntitlementsByUserID(ctx context.Context, userID string, limit, offset int) ([]model.UserEntitlement, int64, error) {
	var entitlements []model.UserEntitlement
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.UserEntitlement{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计权益包数量失败: %w", err)
	}

	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("CASE status WHEN 'ACTIVE' THEN 0 WHEN 'EXHAUSTED' THEN 1 WHEN 'EXPIRED' THEN 2 END, created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&entitlements).Error; err != nil {
		return nil, 0, fmt.Errorf("获取用户权益包列表失败: %w", err)
	}

	return entitlements, total, nil
}

func (r *billingRepo) GetEntitlementByID(ctx context.Context, id int64) (*model.UserEntitlement, error) {
	var entitlement model.UserEntitlement
	if err := r.db.WithContext(ctx).First(&entitlement, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("获取权益包失败: %w", err)
	}
	return &entitlement, nil
}

func (r *billingRepo) CreateEntitlement(ctx context.Context, entitlement *model.UserEntitlement) error {
	if err := r.db.WithContext(ctx).Create(entitlement).Error; err != nil {
		return fmt.Errorf("创建权益包失败: %w", err)
	}
	return nil
}

func (r *billingRepo) UpdateEntitlement(ctx context.Context, entitlement *model.UserEntitlement) error {
	if err := r.db.WithContext(ctx).Save(entitlement).Error; err != nil {
		return fmt.Errorf("更新权益包失败: %w", err)
	}
	return nil
}

func (r *billingRepo) CreateBillingTransaction(ctx context.Context, tx *model.BillingTransactionLog) error {
	if err := r.db.WithContext(ctx).Create(tx).Error; err != nil {
		return fmt.Errorf("创建计费流水失败: %w", err)
	}
	return nil
}

func (r *billingRepo) ListBillingTransactionsByUserID(ctx context.Context, userID string, limit, offset int) ([]model.BillingTransactionLog, int64, error) {
	var transactions []model.BillingTransactionLog
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.BillingTransactionLog{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计计费流水数量失败: %w", err)
	}

	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error; err != nil {
		return nil, 0, fmt.Errorf("获取计费流水失败: %w", err)
	}

	return transactions, total, nil
}

func (r *billingRepo) CreateOrder(ctx context.Context, order *model.UserOrder) error {
	if err := r.db.WithContext(ctx).Create(order).Error; err != nil {
		return fmt.Errorf("创建订单失败: %w", err)
	}
	return nil
}

func (r *billingRepo) GetOrderByCasdoorOrderName(ctx context.Context, orderName string) (*model.UserOrder, error) {
	var order model.UserOrder
	if err := r.db.WithContext(ctx).
		Where("casdoor_order_name = ?", orderName).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("获取订单失败: %w", err)
	}
	return &order, nil
}

func (r *billingRepo) GetOrderByID(ctx context.Context, id int64) (*model.UserOrder, error) {
	var order model.UserOrder
	if err := r.db.WithContext(ctx).First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("获取订单失败: %w", err)
	}
	return &order, nil
}

func (r *billingRepo) UpdateOrder(ctx context.Context, order *model.UserOrder) error {
	if err := r.db.WithContext(ctx).Save(order).Error; err != nil {
		return fmt.Errorf("更新订单失败: %w", err)
	}
	return nil
}

func (r *billingRepo) ListOrdersByUserID(ctx context.Context, userID string, limit, offset int) ([]model.UserOrder, int64, error) {
	var orders []model.UserOrder
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&model.UserOrder{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计订单数量失败: %w", err)
	}

	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&orders).Error; err != nil {
		return nil, 0, fmt.Errorf("获取订单列表失败: %w", err)
	}

	return orders, total, nil
}

func (r *billingRepo) CreateTaskBilling(ctx context.Context, taskBilling *model.TaskBilling) error {
	if err := r.db.WithContext(ctx).Create(taskBilling).Error; err != nil {
		return fmt.Errorf("创建任务计费记录失败: %w", err)
	}
	return nil
}

func (r *billingRepo) GetTaskBillingByTaskRef(ctx context.Context, taskRef string) (*model.TaskBilling, error) {
	var taskBilling model.TaskBilling
	if err := r.db.WithContext(ctx).
		Where("job_uuid = ?", taskRef).
		First(&taskBilling).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("获取任务计费记录失败: %w", err)
	}
	return &taskBilling, nil
}

func (r *billingRepo) UpdateTaskBilling(ctx context.Context, taskBilling *model.TaskBilling) error {
	if err := r.db.WithContext(ctx).Save(taskBilling).Error; err != nil {
		return fmt.Errorf("更新任务计费记录失败: %w", err)
	}
	return nil
}

// Freeze freezes entitlements for a task (two-phase commit - freeze phase).
func (r *billingRepo) Freeze(ctx context.Context, userID string, taskRef string, requiredSeconds int64) (*model.TaskBilling, error) {
	var taskBilling *model.TaskBilling

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.TaskBilling
		if err := tx.Where("job_uuid = ?", taskRef).First(&existing).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("检查任务计费记录失败: %w", err)
			}
		} else {
			taskBilling = &existing
			return nil
		}

		var wallet model.UserWallet
		if err := tx.Where("user_id = ?", userID).First(&wallet).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				wallet = model.UserWallet{UserID: userID, Version: 1}
				if err := tx.Create(&wallet).Error; err != nil {
					return fmt.Errorf("创建用户钱包失败: %w", err)
				}
			} else {
				return fmt.Errorf("获取用户钱包失败: %w", err)
			}
		}

		availableBalance := wallet.TotalAvailable - wallet.TotalFrozen
		if availableBalance < requiredSeconds {
			return ErrInsufficientBalance
		}

		var entitlements []model.UserEntitlement
		query := `
			SELECT * FROM user_entitlement
			WHERE user_id = ?
				AND status = 'ACTIVE'
				AND (valid_until IS NULL OR valid_until > NOW())
				AND (total_seconds - used_seconds - frozen_seconds) > 0
			ORDER BY
				CASE WHEN valid_until IS NULL THEN 1 ELSE 0 END,
				valid_until ASC,
				CASE source_type
					WHEN 'GIFT' THEN 1
					WHEN 'SUBSCRIPTION' THEN 2
					WHEN 'TOP_UP' THEN 3
					WHEN 'TRIAL' THEN 4
				END
		`
		if err := tx.Raw(query, userID).Scan(&entitlements).Error; err != nil {
			return fmt.Errorf("获取用户权益包失败: %w", err)
		}

		remaining := requiredSeconds
		frozenDetails := make(model.FrozenDetails, 0)

		for i := range entitlements {
			if remaining <= 0 {
				break
			}
			available := entitlements[i].AvailableSeconds()
			freezeAmount := available
			if freezeAmount > remaining {
				freezeAmount = remaining
			}
			entitlements[i].FrozenSeconds += freezeAmount
			if err := tx.Save(&entitlements[i]).Error; err != nil {
				return fmt.Errorf("更新权益包失败: %w", err)
			}
			frozenDetails = append(frozenDetails, model.FrozenDetail{
				EntitlementID: entitlements[i].ID,
				Seconds:       freezeAmount,
			})
			remaining -= freezeAmount
		}

		if remaining > 0 {
			return errors.New("冻结失败：权益包余额不足")
		}

		wallet.TotalFrozen += requiredSeconds
		if err := tx.Model(&model.UserWallet{}).
			Where("id = ? AND version = ?", wallet.ID, wallet.Version).
			Updates(map[string]interface{}{
				"total_frozen": wallet.TotalFrozen,
				"version":      wallet.Version + 1,
			}).Error; err != nil {
			return fmt.Errorf("更新钱包失败: %w", err)
		}

		taskBilling = &model.TaskBilling{
			TaskRef:       taskRef,
			UserID:        userID,
			JobSeconds:    requiredSeconds,
			BilledSeconds: requiredSeconds,
			Status:        model.TaskBillingStatusProcessing,
			FrozenDetails: frozenDetails,
		}
		if err := tx.Create(taskBilling).Error; err != nil {
			return fmt.Errorf("创建任务计费记录失败: %w", err)
		}

		transaction := &model.BillingTransactionLog{
			UserID:        userID,
			TaskRef:       &taskRef,
			ActionType:    model.BillingActionFreeze,
			AmountSeconds: requiredSeconds,
			BalanceAfter:  wallet.TotalAvailable - wallet.TotalFrozen,
			Description:   fmt.Sprintf("任务冻结 (#%s)", shortRef(taskRef)),
		}
		if err := tx.Create(transaction).Error; err != nil {
			return fmt.Errorf("创建计费流水失败: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return taskBilling, nil
}

// Consume consumes frozen entitlements after task success (two-phase commit - consume phase).
func (r *billingRepo) Consume(ctx context.Context, taskRef string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var taskBilling model.TaskBilling
		if err := tx.Where("job_uuid = ?", taskRef).First(&taskBilling).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("任务计费记录不存在")
			}
			return fmt.Errorf("获取任务计费记录失败: %w", err)
		}

		if taskBilling.Status != model.TaskBillingStatusProcessing {
			return nil
		}

		var wallet model.UserWallet
		if err := tx.Where("user_id = ?", taskBilling.UserID).First(&wallet).Error; err != nil {
			return fmt.Errorf("获取用户钱包失败: %w", err)
		}

		for _, detail := range taskBilling.FrozenDetails {
			var entitlement model.UserEntitlement
			if err := tx.First(&entitlement, detail.EntitlementID).Error; err != nil {
				return fmt.Errorf("获取权益包失败: %w", err)
			}
			entitlement.FrozenSeconds -= detail.Seconds
			entitlement.UsedSeconds += detail.Seconds
			if entitlement.AvailableSeconds() <= 0 {
				entitlement.Status = model.EntitlementStatusExhausted
			}
			if err := tx.Save(&entitlement).Error; err != nil {
				return fmt.Errorf("更新权益包失败: %w", err)
			}
		}

		wallet.TotalAvailable -= taskBilling.BilledSeconds
		wallet.TotalFrozen -= taskBilling.BilledSeconds
		if err := tx.Model(&model.UserWallet{}).
			Where("id = ? AND version = ?", wallet.ID, wallet.Version).
			Updates(map[string]interface{}{
				"total_available": wallet.TotalAvailable,
				"total_frozen":    wallet.TotalFrozen,
				"version":         wallet.Version + 1,
			}).Error; err != nil {
			return fmt.Errorf("更新钱包失败: %w", err)
		}

		taskBilling.Status = model.TaskBillingStatusSuccess
		if err := tx.Save(&taskBilling).Error; err != nil {
			return fmt.Errorf("更新任务计费状态失败: %w", err)
		}

		transaction := &model.BillingTransactionLog{
			UserID:        taskBilling.UserID,
			TaskRef:       &taskRef,
			ActionType:    model.BillingActionConsume,
			AmountSeconds: taskBilling.BilledSeconds,
			BalanceAfter:  wallet.TotalAvailable - wallet.TotalFrozen,
			Description:   fmt.Sprintf("翻译消费 (#%s)", shortRef(taskRef)),
		}
		if err := tx.Create(transaction).Error; err != nil {
			return fmt.Errorf("创建计费流水失败: %w", err)
		}

		return nil
	})
}

// Unfreeze returns frozen entitlements after task failure (two-phase commit - rollback phase).
func (r *billingRepo) Unfreeze(ctx context.Context, taskRef string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var taskBilling model.TaskBilling
		if err := tx.Where("job_uuid = ?", taskRef).First(&taskBilling).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("任务计费记录不存在")
			}
			return fmt.Errorf("获取任务计费记录失败: %w", err)
		}

		if taskBilling.Status != model.TaskBillingStatusProcessing {
			return nil
		}

		var wallet model.UserWallet
		if err := tx.Where("user_id = ?", taskBilling.UserID).First(&wallet).Error; err != nil {
			return fmt.Errorf("获取用户钱包失败: %w", err)
		}

		for _, detail := range taskBilling.FrozenDetails {
			var entitlement model.UserEntitlement
			if err := tx.First(&entitlement, detail.EntitlementID).Error; err != nil {
				return fmt.Errorf("获取权益包失败: %w", err)
			}
			entitlement.FrozenSeconds -= detail.Seconds
			if entitlement.Status == model.EntitlementStatusExhausted && entitlement.AvailableSeconds() > 0 {
				entitlement.Status = model.EntitlementStatusActive
			}
			if err := tx.Save(&entitlement).Error; err != nil {
				return fmt.Errorf("更新权益包失败: %w", err)
			}
		}

		wallet.TotalFrozen -= taskBilling.BilledSeconds
		if err := tx.Model(&model.UserWallet{}).
			Where("id = ? AND version = ?", wallet.ID, wallet.Version).
			Updates(map[string]interface{}{
				"total_frozen": wallet.TotalFrozen,
				"version":      wallet.Version + 1,
			}).Error; err != nil {
			return fmt.Errorf("更新钱包失败: %w", err)
		}

		taskBilling.Status = model.TaskBillingStatusFailed
		if err := tx.Save(&taskBilling).Error; err != nil {
			return fmt.Errorf("更新任务计费状态失败: %w", err)
		}

		transaction := &model.BillingTransactionLog{
			UserID:        taskBilling.UserID,
			TaskRef:       &taskRef,
			ActionType:    model.BillingActionUnfreeze,
			AmountSeconds: taskBilling.BilledSeconds,
			BalanceAfter:  wallet.TotalAvailable - wallet.TotalFrozen,
			Description:   fmt.Sprintf("任务失败解冻 (#%s)", shortRef(taskRef)),
		}
		if err := tx.Create(transaction).Error; err != nil {
			return fmt.Errorf("创建计费流水失败: %w", err)
		}

		return nil
	})
}

// GrantEntitlement grants an entitlement to a user after payment.
func (r *billingRepo) GrantEntitlement(ctx context.Context, userID string, mapping *model.ProductEntitlementMapping, orderID int64) (*model.UserEntitlement, error) {
	var entitlement *model.UserEntitlement

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check max_per_user limit - count ALL historical purchases
		// This includes expired, exhausted, and deleted entitlements
		if mapping.MaxPerUser > 0 {
			var count int64
			// Count from orders instead, as it records all purchase history
			query := `
				SELECT COUNT(*) FROM user_order
				WHERE user_id = ?
					AND casdoor_product_name = ?
					AND status = ?
			`
			if err := tx.Raw(query, userID, mapping.CasdoorProductName, model.OrderStatusPaid).Scan(&count).Error; err != nil {
				return fmt.Errorf("检查购买次数失败: %w", err)
			}
			if count >= int64(mapping.MaxPerUser) {
				return errors.New("已达到该产品的最大购买次数限制")
			}
		}

		var wallet model.UserWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userID).
			First(&wallet).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				wallet = model.UserWallet{UserID: userID, Version: 1}
				if err := tx.Create(&wallet).Error; err != nil {
					return fmt.Errorf("创建用户钱包失败: %w", err)
				}
			} else {
				return fmt.Errorf("获取用户钱包失败: %w", err)
			}
		}

		now := time.Now()
		var validUntil *time.Time
		// Support both PeriodMonths and PeriodDays
		if mapping.PeriodDays > 0 {
			expiry := now.AddDate(0, 0, mapping.PeriodDays)
			validUntil = &expiry
		} else if mapping.PeriodMonths > 0 {
			expiry := now.AddDate(0, mapping.PeriodMonths, 0)
			validUntil = &expiry
		}

		entitlement = &model.UserEntitlement{
			UserID:             userID,
			SourceType:         mapping.EntitlementType,
			TotalSeconds:       mapping.QuotaSeconds,
			ValidFrom:          now,
			ValidUntil:         validUntil,
			Status:             model.EntitlementStatusActive,
			CasdoorProductName: mapping.CasdoorProductName,
			OrderID:            &orderID,
		}
		if err := tx.Create(entitlement).Error; err != nil {
			return fmt.Errorf("创建权益包失败: %w", err)
		}

		wallet.TotalAvailable += mapping.QuotaSeconds
		if err := tx.Model(&model.UserWallet{}).
			Where("id = ? AND version = ?", wallet.ID, wallet.Version).
			Updates(map[string]interface{}{
				"total_available": wallet.TotalAvailable,
				"version":         wallet.Version + 1,
			}).Error; err != nil {
			return fmt.Errorf("更新钱包失败: %w", err)
		}

		transaction := &model.BillingTransactionLog{
			UserID:        userID,
			EntitlementID: &entitlement.ID,
			ActionType:    model.BillingActionGrant,
			AmountSeconds: mapping.QuotaSeconds,
			BalanceAfter:  wallet.TotalAvailable - wallet.TotalFrozen,
			Description:   grantDescription(mapping),
		}
		if err := tx.Create(transaction).Error; err != nil {
			return fmt.Errorf("创建计费流水失败: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return entitlement, nil
}

// ExpireEntitlements processes expired entitlements in batches and adjusts wallet balances.
func (r *billingRepo) ExpireEntitlements(ctx context.Context, batchSize int) (expired int64, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find entitlements that are ACTIVE but past valid_until
		var entitlements []model.UserEntitlement
		query := `
			SELECT * FROM user_entitlement
			WHERE status = 'ACTIVE'
				AND valid_until IS NOT NULL
				AND valid_until <= NOW()
			ORDER BY valid_until ASC
			LIMIT ?
		`
		if err := tx.Raw(query, batchSize).Scan(&entitlements).Error; err != nil {
			return fmt.Errorf("查询过期权益包失败: %w", err)
		}

		for _, e := range entitlements {
			// Calculate remaining available seconds
			remaining := e.AvailableSeconds()
			if remaining < 0 {
				remaining = 0
			}

			// Update entitlement status
			e.Status = model.EntitlementStatusExpired
			if err := tx.Save(&e).Error; err != nil {
				return fmt.Errorf("更新权益包状态失败: %w", err)
			}

			// Deduct from wallet
			if remaining > 0 {
				var wallet model.UserWallet
				if err := tx.Where("user_id = ?", e.UserID).First(&wallet).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue // No wallet to update
					}
					return fmt.Errorf("获取钱包失败: %w", err)
				}

				wallet.TotalAvailable -= remaining
				if wallet.TotalAvailable < 0 {
					wallet.TotalAvailable = 0
				}

				if err := tx.Model(&model.UserWallet{}).
					Where("id = ? AND version = ?", wallet.ID, wallet.Version).
					Updates(map[string]interface{}{
						"total_available": wallet.TotalAvailable,
						"version":         wallet.Version + 1,
					}).Error; err != nil {
					return fmt.Errorf("更新钱包失败: %w", err)
				}

				// Record expiration transaction
				transaction := &model.BillingTransactionLog{
					UserID:        e.UserID,
					EntitlementID: &e.ID,
					ActionType:    model.BillingActionExpire,
					AmountSeconds: -remaining,
					BalanceAfter:  wallet.TotalAvailable - wallet.TotalFrozen,
					Description:   fmt.Sprintf("权益包过期: %s (剩余%d秒)", e.CasdoorProductName, remaining),
				}
				if err := tx.Create(transaction).Error; err != nil {
					return fmt.Errorf("创建计费流水失败: %w", err)
				}
			}

			expired++
		}

		return nil
	})

	return expired, err
}

// ReconcileWallet recalculates wallet balance from active entitlements.
func (r *billingRepo) ReconcileWallet(ctx context.Context, userID string) (before, after int64, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var wallet model.UserWallet
		if err := tx.Where("user_id = ?", userID).First(&wallet).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				wallet = model.UserWallet{UserID: userID, Version: 1}
				if err := tx.Create(&wallet).Error; err != nil {
					return fmt.Errorf("创建钱包失败: %w", err)
				}
			} else {
				return fmt.Errorf("获取钱包失败: %w", err)
			}
		}

		before = wallet.TotalAvailable

		// Calculate correct balance from all active entitlements
		var calculatedBalance int64
		query := `
			SELECT COALESCE(SUM(total_seconds - used_seconds - frozen_seconds), 0)
			FROM user_entitlement
			WHERE user_id = ?
				AND status = 'ACTIVE'
				AND (valid_until IS NULL OR valid_until > NOW())
		`
		if err := tx.Raw(query, userID).Scan(&calculatedBalance).Error; err != nil {
			return fmt.Errorf("计算权益包余额失败: %w", err)
		}

		after = calculatedBalance

		// Update wallet if different
		if wallet.TotalAvailable != calculatedBalance {
			wallet.TotalAvailable = calculatedBalance
			if err := tx.Model(&model.UserWallet{}).
				Where("id = ? AND version = ?", wallet.ID, wallet.Version).
				Updates(map[string]interface{}{
					"total_available": wallet.TotalAvailable,
					"version":         wallet.Version + 1,
				}).Error; err != nil {
				return fmt.Errorf("更新钱包失败: %w", err)
			}
		}

		return nil
	})

	return before, after, err
}

// ListUsersWithWalletDiscrepancy finds users with mismatched wallets.
func (r *billingRepo) ListUsersWithWalletDiscrepancy(ctx context.Context, limit int) ([]string, error) {
	query := `
		SELECT DISTINCT w.user_id
		FROM user_wallet w
		LEFT JOIN (
			SELECT user_id,
				   SUM(total_seconds - used_seconds - frozen_seconds) as calculated
			FROM user_entitlement
			WHERE status = 'ACTIVE'
				AND (valid_until IS NULL OR valid_until > NOW())
			GROUP BY user_id
		) e ON w.user_id = e.user_id
		WHERE w.total_available != COALESCE(e.calculated, 0)
		LIMIT ?
	`

	var userIDs []string
	if err := r.db.WithContext(ctx).Raw(query, limit).Scan(&userIDs).Error; err != nil {
		return nil, fmt.Errorf("查询钱包不一致用户失败: %w", err)
	}

	return userIDs, nil
}

// CanUserPurchaseProduct checks if a user can still purchase a product based on max_per_user limit.
func (r *billingRepo) CanUserPurchaseProduct(ctx context.Context, userID string, productName string) (bool, error) {
	// Get the product mapping to check max_per_user
	mapping, err := r.GetProductMappingByProductName(ctx, productName)
	if err != nil {
		return false, fmt.Errorf("获取产品映射失败: %w", err)
	}
	if mapping == nil {
		return false, fmt.Errorf("产品映射不存在: %s", productName)
	}

	// If no max_per_user limit, user can always purchase
	if mapping.MaxPerUser <= 0 {
		return true, nil
	}

	// Count all successful purchases for this user and product
	var count int64
	query := `
		SELECT COUNT(*) FROM user_order
		WHERE user_id = ?
			AND casdoor_product_name = ?
			AND status = ?
	`
	if err := r.db.WithContext(ctx).Raw(query, userID, productName, model.OrderStatusPaid).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("查询购买次数失败: %w", err)
	}

	return count < int64(mapping.MaxPerUser), nil
}

// shortRef returns the first 8 characters of a reference for display.
func shortRef(ref string) string {
	if len(ref) > 8 {
		return ref[:8]
	}
	return ref
}

// grantDescription returns a user-friendly description for a grant action.
func grantDescription(mapping *model.ProductEntitlementMapping) string {
	if mapping.Description != "" {
		return mapping.Description
	}
	return fmt.Sprintf("充值时长: %s", mapping.CasdoorProductName)
}
