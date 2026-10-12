package service

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PaymentYooKassa = "yookassa"
	PaymentPlatega  = "platega"
	PaymentStars    = "stars"

	PaymentPending = "pending"
	PaymentPaid    = "paid"
	PaymentFailed  = "failed"

	FulfillmentPending   = "pending"
	FulfillmentRunning   = "processing"
	FulfillmentComplete  = "completed"
	FulfillmentFailed    = "failed"
	planTrafficUnitBytes = int64(1 << 30)
)

type PaymentProviderView struct {
	Provider   string `json:"provider"`
	Enabled    bool   `json:"enabled"`
	MerchantID string `json:"merchantId"`
	HasSecret  bool   `json:"hasSecret"`
}

type PaymentProviderInput struct {
	Provider   string `json:"provider"`
	Enabled    bool   `json:"enabled"`
	MerchantID string `json:"merchantId"`
	Secret     string `json:"secret"`
	ClearSecret bool  `json:"clearSecret"`
}

type PaymentService struct {
	ClientService ClientService
	InboundService InboundService
}

type paymentClientFulfillmentLock struct {
	mu   sync.Mutex
	refs int
}

var paymentClientFulfillmentLocks = struct {
	sync.Mutex
	locks map[string]*paymentClientFulfillmentLock
}{locks: make(map[string]*paymentClientFulfillmentLock)}

func lockPaymentClient(key string) func() {
	paymentClientFulfillmentLocks.Lock()
	lock := paymentClientFulfillmentLocks.locks[key]
	if lock == nil {
		lock = &paymentClientFulfillmentLock{}
		paymentClientFulfillmentLocks.locks[key] = lock
	}
	lock.refs++
	paymentClientFulfillmentLocks.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		paymentClientFulfillmentLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(paymentClientFulfillmentLocks.locks, key)
		}
		paymentClientFulfillmentLocks.Unlock()
	}
}

func knownPaymentProvider(provider string) bool {
	switch provider {
	case PaymentYooKassa, PaymentPlatega, PaymentStars:
		return true
	default:
		return false
	}
}

func (s *PaymentService) GetProviderConfigs() ([]PaymentProviderView, error) {
	providers := []string{PaymentPlatega, PaymentYooKassa, PaymentStars}
	views := make([]PaymentProviderView, 0, len(providers))
	db := database.GetDB()
	for _, provider := range providers {
		var cfg model.PaymentProviderConfig
		err := db.Where("provider = ?", provider).First(&cfg).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cfg.Provider = provider
			if err := db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "provider"}},
				DoNothing: true,
			}).Create(&cfg).Error; err != nil {
				return nil, err
			}
			if err := db.Where("provider = ?", provider).First(&cfg).Error; err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		views = append(views, PaymentProviderView{
			Provider: provider, Enabled: cfg.Enabled, MerchantID: cfg.MerchantID,
			HasSecret: strings.TrimSpace(cfg.Secret) != "",
		})
	}
	return views, nil
}

func (s *PaymentService) UpdateProviderConfigs(inputs []PaymentProviderInput) ([]PaymentProviderView, error) {
	db := database.GetDB()
	err := db.Transaction(func(tx *gorm.DB) error {
		seen := make(map[string]bool, len(inputs))
		configs := make([]model.PaymentProviderConfig, 0, len(inputs))
		for _, input := range inputs {
			input.Provider = strings.TrimSpace(strings.ToLower(input.Provider))
			if !knownPaymentProvider(input.Provider) || seen[input.Provider] {
				return fmt.Errorf("unknown or repeated payment provider %q", input.Provider)
			}
			seen[input.Provider] = true
			var cfg model.PaymentProviderConfig
			err := tx.Where("provider = ?", input.Provider).First(&cfg).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				cfg.Provider = input.Provider
			} else if err != nil {
				return err
			}
			cfg.Enabled = input.Enabled
			cfg.MerchantID = strings.TrimSpace(input.MerchantID)
			if input.Secret != "" {
				cfg.Secret = strings.TrimSpace(input.Secret)
			} else if input.ClearSecret {
				cfg.Secret = ""
			}
			if cfg.Enabled {
				switch cfg.Provider {
				case PaymentYooKassa:
					if cfg.MerchantID == "" || cfg.Secret == "" {
						return errors.New("YooKassa requires a shop ID and secret key before it can be enabled")
					}
				case PaymentPlatega:
					if cfg.MerchantID == "" || cfg.Secret == "" {
						return errors.New("Platega requires a merchant ID and API secret before it can be enabled")
					}
				}
			}
			configs = append(configs, cfg)
		}
		for i := range configs {
			if err := tx.Save(&configs[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetProviderConfigs()
}

func (s *PaymentService) storedProviderConfig(provider string) (*model.PaymentProviderConfig, error) {
	if !knownPaymentProvider(provider) {
		return nil, fmt.Errorf("unknown payment provider %q", provider)
	}
	var cfg model.PaymentProviderConfig
	if err := database.GetDB().Where("provider = ?", provider).First(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *PaymentService) providerConfig(provider string) (*model.PaymentProviderConfig, error) {
	cfg, err := s.storedProviderConfig(provider)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("%s payments are disabled", provider)
	}
	return &cfg, nil
}

func (s *PaymentService) GetProviderConfig(provider string) (*model.PaymentProviderConfig, error) {
	if !knownPaymentProvider(provider) {
		return nil, fmt.Errorf("unknown payment provider %q", provider)
	}
	return s.providerConfig(provider)
}

func (s *PaymentService) CreateOrder(userID int, tgID int64, planID, clientID int, provider string) (*model.PaymentOrder, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if !knownPaymentProvider(provider) || tgID < 1 {
		return nil, errors.New("invalid customer or payment provider")
	}
	if _, err := s.providerConfig(provider); err != nil {
		return nil, err
	}
	db := database.GetDB()
	var plan model.Plan
	if err := db.Where("user_id = ? AND id = ? AND enabled = ?", userID, planID, true).First(&plan).Error; err != nil {
		return nil, err
	}
	if len(plan.InboundIds) == 0 {
		return nil, errors.New("plan currently has no available locations")
	}
	var inboundCount int64
	if err := db.Model(&model.Inbound{}).Where("user_id = ? AND id IN ? AND enable = ?", userID, plan.InboundIds, true).Count(&inboundCount).Error; err != nil {
		return nil, err
	}
	if inboundCount != int64(len(plan.InboundIds)) {
		return nil, errors.New("one or more plan locations are unavailable")
	}
	var client *model.ClientRecord
	if clientID > 0 {
		client = &model.ClientRecord{}
		if err := db.Where("id = ? AND tg_id = ?", clientID, tgID).First(client).Error; err != nil {
			return nil, errors.New("the selected VPN profile does not belong to this Telegram account")
		}
	} else {
		var owned int64
		if err := db.Model(&model.ClientRecord{}).Where("tg_id = ?", tgID).Count(&owned).Error; err != nil {
			return nil, err
		}
		if owned > 0 {
			return nil, errors.New("select one of your existing VPN profiles")
		}
	}
	amount, currency := plan.YooKassaPriceKopecks, "RUB"
	switch provider {
	case PaymentPlatega:
		amount = plan.PlategaPriceKopecks
	case PaymentStars:
		amount, currency = plan.StarsPrice, "XTR"
	}
	order := &model.PaymentOrder{
		UserId: userID, UID: uuid.NewString(), TgID: tgID, PlanId: plan.Id,
		PlanName: plan.Name, PlanDescription: plan.Description, DurationDays: plan.DurationDays,
		TrafficLimitBytes: plan.TrafficLimitBytes, DeviceLimit: plan.DeviceLimit,
		InboundIds: append([]int(nil), plan.InboundIds...), Provider: provider,
		Amount: amount, Currency: currency, Status: PaymentPending,
		FulfillmentStatus: FulfillmentPending,
	}
	if client != nil {
		order.ClientId = client.Id
		order.ClientEmail = client.Email
		order.ClientSubID = client.SubID
	} else {
		order.ClientSubID = uuid.NewString()
	}
	if err := db.Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

func (s *PaymentService) SetCheckout(orderID int, providerPaymentID, paymentURL string) error {
	return database.GetDB().Model(&model.PaymentOrder{}).
		Where("id = ? AND status = ?", orderID, PaymentPending).
		Updates(map[string]any{"provider_payment_id": providerPaymentID, "payment_url": paymentURL, "updated_at": time.Now().UnixMilli()}).Error
}

func (s *PaymentService) FailCheckout(orderID int, cause error) error {
	if cause == nil {
		return nil
	}
	return database.GetDB().Model(&model.PaymentOrder{}).
		Where("id = ? AND status = ?", orderID, PaymentPending).
		Updates(map[string]any{"status": PaymentFailed, "error": cause.Error(), "updated_at": time.Now().UnixMilli()}).Error
}

func (s *PaymentService) GetOrder(uid string) (*model.PaymentOrder, error) {
	var order model.PaymentOrder
	err := database.GetDB().Where("uid = ?", uid).First(&order).Error
	return &order, err
}

func (s *PaymentService) GetCustomerOrder(uid string, tgID int64) (*model.PaymentOrder, error) {
	var order model.PaymentOrder
	err := database.GetDB().Where("uid = ? AND tg_id = ?", uid, tgID).First(&order).Error
	return &order, err
}

func (s *PaymentService) ListOrders(userID, limit, offset int) ([]model.PaymentOrder, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var orders []model.PaymentOrder
	err := database.GetDB().Where("user_id = ?", userID).Order("created_at desc, id desc").Limit(limit).Offset(offset).Find(&orders).Error
	return orders, err
}

func (s *PaymentService) ValidateStarsOrder(uid string, tgID int64, amount int64) (*model.PaymentOrder, error) {
	order, err := s.GetCustomerOrder(uid, tgID)
	if err != nil {
		return nil, err
	}
	if order.Provider != PaymentStars || order.Currency != "XTR" || order.Amount != amount || order.Status != PaymentPending {
		return nil, errors.New("Telegram Stars invoice does not match this order")
	}
	return order, nil
}

func (s *PaymentService) ConfirmStarsOrder(uid string, tgID int64, amount int64, chargeID string) error {
	order, err := s.ValidateStarsOrder(uid, tgID, amount)
	if err != nil {
		// Telegram can redeliver successful_payment after the first handler has
		// finished, so an already-paid matching charge is an idempotent success.
		if existing, lookupErr := s.GetCustomerOrder(uid, tgID); lookupErr == nil && existing.Provider == PaymentStars && existing.Amount == amount && existing.Status == PaymentPaid && existing.TelegramChargeID == chargeID {
			return s.FulfillOrder(uid)
		}
		return err
	}
	if err := s.markPaid(order, chargeID); err != nil {
		return err
	}
	return s.FulfillOrder(uid)
}

func (s *PaymentService) ConfirmProviderOrder(uid, provider, providerPaymentID, currency string, amount int64) error {
	order, err := s.GetOrder(uid)
	if err != nil {
		return err
	}
	if order.Provider != provider || order.Currency != currency || order.Amount != amount || order.ProviderPaymentID != providerPaymentID {
		return errors.New("provider payment does not match the saved order")
	}
	if order.Status == PaymentPaid {
		return s.FulfillOrder(uid)
	}
	if order.Status != PaymentPending {
		return fmt.Errorf("order is %s", order.Status)
	}
	if err := s.markPaid(order, providerPaymentID); err != nil {
		return err
	}
	return s.FulfillOrder(uid)
}

func (s *PaymentService) markPaid(order *model.PaymentOrder, chargeID string) error {
	now := time.Now().UnixMilli()
	result := database.GetDB().Model(&model.PaymentOrder{}).
		Where("id = ? AND status = ?", order.Id, PaymentPending).
		Updates(map[string]any{"status": PaymentPaid, "paid_at": now, "telegram_charge_id": chargeID, "error": "", "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var current model.PaymentOrder
		if err := database.GetDB().First(&current, order.Id).Error; err != nil {
			return err
		}
		if current.Status != PaymentPaid {
			return fmt.Errorf("order is %s", current.Status)
		}
	}
	return nil
}

func (s *PaymentService) FulfillOrder(uid string) error {
	var order model.PaymentOrder
	if err := database.GetDB().Where("uid = ?", uid).First(&order).Error; err != nil {
		return err
	}
	claimed := false
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var current model.PaymentOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", order.Id).First(&current).Error; err != nil {
			return err
		}
		if current.Status != PaymentPaid || current.FulfillmentStatus == FulfillmentComplete {
			return nil
		}
		if current.FulfillmentStatus == FulfillmentRunning && time.Now().UnixMilli()-current.UpdatedAt < int64(5*time.Minute/time.Millisecond) {
			return nil
		}
		if err := tx.Model(&current).Updates(map[string]any{"fulfillment_status": FulfillmentRunning, "updated_at": time.Now().UnixMilli()}).Error; err != nil {
			return err
		}
		order = current
		claimed = true
		return nil
	})
	if err != nil || !claimed {
		return err
	}
	fulfillErr := s.applyPlanToClient(&order)
	if fulfillErr != nil {
		_ = database.GetDB().Model(&model.PaymentOrder{}).Where("id = ?", order.Id).
			Updates(map[string]any{"fulfillment_status": FulfillmentFailed, "error": fulfillErr.Error(), "updated_at": time.Now().UnixMilli()}).Error
		return fulfillErr
	}
	return database.GetDB().Model(&model.PaymentOrder{}).Where("id = ?", order.Id).
		Updates(map[string]any{"fulfillment_status": FulfillmentComplete, "error": "", "fulfilled_at": time.Now().UnixMilli(), "updated_at": time.Now().UnixMilli()}).Error
}

func (s *PaymentService) applyPlanToClient(order *model.PaymentOrder) error {
	lockKey := fmt.Sprintf("%d:%d", order.UserId, order.TgID)
	unlock := lockPaymentClient(lockKey)
	defer unlock()

	if len(order.InboundIds) == 0 {
		return errors.New("the purchased plan no longer contains any locations")
	}
	db := database.GetDB()
	var inbounds []model.Inbound
	if err := db.Where("user_id = ? AND id IN ? AND enable = ?", order.UserId, order.InboundIds, true).Find(&inbounds).Error; err != nil {
		return err
	}
	if len(inbounds) != len(order.InboundIds) {
		return errors.New("one or more locations in this paid order were removed or disabled; contact the operator")
	}
	if order.TrafficLimitBytes%planTrafficUnitBytes != 0 {
		return errors.New("this plan's traffic allowance is not supported by the client core")
	}
	if order.ClientId == 0 {
		if order.ClientEmail == "" {
			order.ClientEmail = fmt.Sprintf("store-%d-%s", order.TgID, strings.ReplaceAll(order.UID, "-", ""))
		}
		if order.ClientSubID == "" {
			order.ClientSubID = uuid.NewString()
		}
		if order.TargetExpiry == 0 {
			order.TargetExpiry = order.PaidAt + int64(order.DurationDays)*24*60*60*1000
		}
		if err := db.Model(&model.PaymentOrder{}).Where("id = ?", order.Id).Updates(map[string]any{
			"client_email": order.ClientEmail, "client_sub_id": order.ClientSubID,
			"target_expiry": order.TargetExpiry,
		}).Error; err != nil {
			return err
		}
		client := model.Client{
			Email: order.ClientEmail, SubID: order.ClientSubID, TgID: order.TgID, Enable: true,
			TotalGB: order.TrafficLimitBytes / planTrafficUnitBytes, ExpiryTime: order.TargetExpiry,
			LimitIP: 0, Comment: "Telegram plan: " + order.PlanName,
		}
		now := time.Now().UnixMilli()
		client.CreatedAt = now
		client.UpdatedAt = now
		if _, err := s.ClientService.Create(&s.InboundService, &ClientCreatePayload{Client: client, InboundIds: order.InboundIds, LimitHwid: order.DeviceLimit}); err != nil {
			return err
		}
		record, err := s.ClientService.GetRecordByEmail(nil, order.ClientEmail)
		if err != nil {
			return err
		}
		order.ClientId = record.Id
		if err := db.Model(&model.PaymentOrder{}).Where("id = ?", order.Id).
			Updates(map[string]any{"client_id": record.Id, "client_email": record.Email, "client_sub_id": record.SubID}).Error; err != nil {
			return err
		}
		return nil
	}
	var existing model.ClientRecord
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tg_id = ?", order.ClientId, order.TgID).First(&existing).Error; err != nil {
			return err
		}
		var currentOrder model.PaymentOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentOrder, order.Id).Error; err != nil {
			return err
		}
		if currentOrder.TargetExpiry == 0 {
			base := max(existing.ExpiryTime, currentOrder.PaidAt)
			currentOrder.TargetExpiry = base + int64(currentOrder.DurationDays)*24*60*60*1000
			if err := tx.Model(&currentOrder).Update("target_expiry", currentOrder.TargetExpiry).Error; err != nil {
				return err
			}
			if err := tx.Model(&existing).Update("expiry_time", currentOrder.TargetExpiry).Error; err != nil {
				return err
			}
		}
		order.TargetExpiry = currentOrder.TargetExpiry
		return nil
	})
	if err != nil {
		return errors.New("the selected VPN profile is no longer available for this Telegram account")
	}
	client := *existing.ToClient()
	client.Enable = true
	client.TgID = order.TgID
	client.TotalGB = order.TrafficLimitBytes / planTrafficUnitBytes
	client.ExpiryTime = order.TargetExpiry
	currentIDs, err := s.ClientService.GetInboundIdsForRecord(existing.Id)
	if err != nil {
		return err
	}
	removeIDs := make([]int, 0)
	addIDs := make([]int, 0)
	for _, id := range currentIDs {
		if !slices.Contains(order.InboundIds, id) {
			removeIDs = append(removeIDs, id)
		}
	}
	for _, id := range order.InboundIds {
		if !slices.Contains(currentIDs, id) {
			addIDs = append(addIDs, id)
		}
	}
	if len(removeIDs) > 0 {
		if _, err := s.ClientService.Detach(&s.InboundService, existing.Id, removeIDs); err != nil {
			return err
		}
	}
	if len(addIDs) > 0 {
		if _, err := s.ClientService.Attach(&s.InboundService, existing.Id, addIDs); err != nil {
			return err
		}
	}
	if _, err := s.ClientService.Update(&s.InboundService, existing.Id, client, order.DeviceLimit); err != nil {
		return err
	}
	if _, err := s.ClientService.ResetTrafficByEmail(&s.InboundService, existing.Email); err != nil {
		return err
	}
	return db.Model(&model.PaymentOrder{}).Where("id = ?", order.Id).
		Updates(map[string]any{"client_email": existing.Email, "client_sub_id": existing.SubID}).Error
}

func rublesToKopecks(amount float64) (int64, bool) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return 0, false
	}
	kopecks := math.Round(amount * 100)
	if math.Abs(amount*100-kopecks) > 0.0001 {
		return 0, false
	}
	return int64(kopecks), true
}
