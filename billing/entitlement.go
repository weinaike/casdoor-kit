package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/weinaike/casdoor-kit"
	"github.com/weinaike/casdoor-kit/billing/model"
	"github.com/weinaike/casdoor-kit/billing/repo"
	"github.com/weinaike/casdoor-kit/config"
)

// EntitlementService manages user entitlements and billing.
type EntitlementService interface {
	GetWallet(ctx context.Context, userID string) (*UserWalletInfo, error)
	ListEntitlements(ctx context.Context, userID string, limit, offset int) ([]EntitlementInfo, int64, error)
	FreezeForTask(ctx context.Context, userID string, taskRef string, requiredSeconds int64) error
	ConsumeTask(ctx context.Context, taskRef string) error
	UnfreezeTask(ctx context.Context, taskRef string) error
	GrantEntitlement(ctx context.Context, userID string, productName string, orderID int64) (*model.UserEntitlement, error)
	GetBillingHistory(ctx context.Context, userID string, limit, offset int) ([]BillingHistoryEntry, int64, error)
}

// UserWalletInfo is the wallet info returned to the frontend.
type UserWalletInfo struct {
	TotalSeconds      int64 `json:"total_seconds"`
	FrozenSeconds     int64 `json:"frozen_seconds"`
	AvailableSeconds  int64 `json:"available_seconds"`
	EntitlementsCount int   `json:"entitlements_count"`
}

// EntitlementInfo is the entitlement info returned to the frontend.
type EntitlementInfo struct {
	ID                 int64                   `json:"id"`
	SourceType         model.EntitlementType   `json:"source_type"`
	TotalSeconds       int64                   `json:"total_seconds"`
	UsedSeconds        int64                   `json:"used_seconds"`
	AvailableSeconds   int64                   `json:"available_seconds"`
	ValidFrom          string                  `json:"valid_from"`
	ValidUntil         *string                 `json:"valid_until,omitempty"`
	Status             model.EntitlementStatus `json:"status"`
	CasdoorProductName string                  `json:"casdoor_product_name"`
	DisplayName        string                  `json:"display_name"`
	CreatedAt          string                  `json:"created_at"`
}

// BillingHistoryEntry is a billing history entry returned to the frontend.
type BillingHistoryEntry struct {
	ID            int64                   `json:"id"`
	ActionType    model.BillingActionType `json:"action_type"`
	AmountSeconds int64                   `json:"amount_seconds"`
	BalanceAfter  int64                   `json:"balance_after"`
	Description   string                  `json:"description"`
	JobUUID       *string                 `json:"job_uuid,omitempty"`
	CreatedAt     string                  `json:"created_at"`
}

type entitlementService struct {
	billingRepo repo.BillingRepository
	config      *config.EntitlementConfig
}

// NewEntitlementService creates an entitlement service.
func NewEntitlementService(billingRepo repo.BillingRepository, cfg *config.EntitlementConfig) EntitlementService {
	return &entitlementService{
		billingRepo: billingRepo,
		config:      cfg,
	}
}

func (s *entitlementService) GetWallet(ctx context.Context, userID string) (*UserWalletInfo, error) {
	wallet, err := s.billingRepo.GetWalletByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("获取用户钱包失败: %w", err)
	}

	if wallet == nil {
		return &UserWalletInfo{}, nil
	}

	entitlements, err := s.billingRepo.GetActiveEntitlementsByUserID(ctx, userID)
	if err != nil {
		gokit.GetLogger().Warn("获取用户权益包失败", "error", err, "user_id", userID)
	}

	return &UserWalletInfo{
		TotalSeconds:      wallet.TotalAvailable,
		FrozenSeconds:     wallet.TotalFrozen,
		AvailableSeconds:  wallet.TotalAvailable - wallet.TotalFrozen,
		EntitlementsCount: len(entitlements),
	}, nil
}

func (s *entitlementService) ListEntitlements(ctx context.Context, userID string, limit, offset int) ([]EntitlementInfo, int64, error) {
	entitlements, total, err := s.billingRepo.ListEntitlementsByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("获取权益包列表失败: %w", err)
	}

	mappings, _ := s.billingRepo.ListActiveProductMappings(ctx)
	displayNames := make(map[string]string, len(mappings))
	for _, m := range mappings {
		displayNames[m.CasdoorProductName] = m.Description
	}

	items := make([]EntitlementInfo, 0, len(entitlements))
	for _, e := range entitlements {
		var validUntil *string
		if e.ValidUntil != nil {
			s := e.ValidUntil.Format("2006-01-02T15:04:05Z07:00")
			validUntil = &s
		}
		displayName := e.CasdoorProductName
		if desc, ok := displayNames[e.CasdoorProductName]; ok && desc != "" {
			displayName = desc
		}
		items = append(items, EntitlementInfo{
			ID:                 e.ID,
			SourceType:         e.SourceType,
			TotalSeconds:       e.TotalSeconds,
			UsedSeconds:        e.UsedSeconds,
			AvailableSeconds:   e.TotalSeconds - e.UsedSeconds - e.FrozenSeconds,
			ValidFrom:          e.ValidFrom.Format("2006-01-02T15:04:05Z07:00"),
			ValidUntil:         validUntil,
			Status:             e.Status,
			CasdoorProductName: e.CasdoorProductName,
			DisplayName:        displayName,
			CreatedAt:          e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return items, total, nil
}

func (s *entitlementService) FreezeForTask(ctx context.Context, userID string, taskRef string, requiredSeconds int64) error {
	if requiredSeconds <= 0 {
		return errors.New("冻结时长必须大于0")
	}

	taskBilling, err := s.billingRepo.Freeze(ctx, userID, taskRef, requiredSeconds)
	if err != nil {
		if errors.Is(err, repo.ErrInsufficientBalance) {
			return errors.New("余额不足，请充值后重试")
		}
		return fmt.Errorf("冻结权益失败: %w", err)
	}

	gokit.GetLogger().Info("权益冻结成功",
		"user_id", userID,
		"task_ref", taskRef,
		"seconds", requiredSeconds,
		"entitlements_affected", len(taskBilling.FrozenDetails))

	return nil
}

func (s *entitlementService) ConsumeTask(ctx context.Context, taskRef string) error {
	if err := s.billingRepo.Consume(ctx, taskRef); err != nil {
		return fmt.Errorf("核销权益失败: %w", err)
	}
	gokit.GetLogger().Info("权益核销成功", "task_ref", taskRef)
	return nil
}

func (s *entitlementService) UnfreezeTask(ctx context.Context, taskRef string) error {
	if err := s.billingRepo.Unfreeze(ctx, taskRef); err != nil {
		return fmt.Errorf("解冻权益失败: %w", err)
	}
	gokit.GetLogger().Info("权益解冻成功", "task_ref", taskRef)
	return nil
}

func (s *entitlementService) GrantEntitlement(ctx context.Context, userID string, productName string, orderID int64) (*model.UserEntitlement, error) {
	var mapping *model.ProductEntitlementMapping

	if s.config != nil && s.config.ProductMappings != nil {
		if cfgMapping, ok := s.config.ProductMappings[productName]; ok {
			entitlementType := model.EntitlementTypeTopUp
			if cfgMapping.EntitlementType == "SUBSCRIPTION" {
				entitlementType = model.EntitlementTypeSubscription
			} else if cfgMapping.EntitlementType == "GIFT" {
				entitlementType = model.EntitlementTypeGift
			}
			mapping = &model.ProductEntitlementMapping{
				CasdoorProductName: productName,
				QuotaSeconds:       cfgMapping.QuotaSeconds,
				EntitlementType:    entitlementType,
				PeriodMonths:       cfgMapping.PeriodMonths,
				Description:        cfgMapping.Description,
				IsActive:           true,
			}
		}
	}

	if mapping == nil {
		var err error
		mapping, err = s.billingRepo.GetProductMappingByProductName(ctx, productName)
		if err != nil {
			return nil, fmt.Errorf("获取产品权益映射失败: %w", err)
		}
		if mapping == nil {
			return nil, fmt.Errorf("产品权益映射不存在: %s", productName)
		}
	}

	entitlement, err := s.billingRepo.GrantEntitlement(ctx, userID, mapping, orderID)
	if err != nil {
		return nil, fmt.Errorf("发放权益失败: %w", err)
	}

	gokit.GetLogger().Info("权益发放成功",
		"user_id", userID,
		"product_name", productName,
		"quota_seconds", mapping.QuotaSeconds,
		"entitlement_id", entitlement.ID)

	return entitlement, nil
}

func (s *entitlementService) GetBillingHistory(ctx context.Context, userID string, limit, offset int) ([]BillingHistoryEntry, int64, error) {
	transactions, total, err := s.billingRepo.ListBillingTransactionsByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("获取计费历史失败: %w", err)
	}

	entries := make([]BillingHistoryEntry, 0, len(transactions))
	for _, tx := range transactions {
		entries = append(entries, BillingHistoryEntry{
			ID:            tx.ID,
			ActionType:    tx.ActionType,
			AmountSeconds: tx.AmountSeconds,
			BalanceAfter:  tx.BalanceAfter,
			Description:   tx.Description,
			JobUUID:       tx.TaskRef,
			CreatedAt:     tx.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return entries, total, nil
}
