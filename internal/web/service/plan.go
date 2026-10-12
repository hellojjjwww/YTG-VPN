package service

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"gorm.io/gorm"
)

type PlanService struct{}

func (s *PlanService) List(userID int) ([]*model.Plan, error) {
	var plans []*model.Plan
	err := database.GetDB().Where("user_id = ?", userID).Order("sort_order asc, id asc").Find(&plans).Error
	return plans, err
}

func (s *PlanService) validate(userID int, plan *model.Plan) error {
	plan.Name = strings.TrimSpace(plan.Name)
	plan.Description = strings.TrimSpace(plan.Description)
	if plan.Name == "" || utf8.RuneCountInString(plan.Name) > 120 {
		return common.NewError("plan name must contain 1 to 120 characters")
	}
	if utf8.RuneCountInString(plan.Description) > 2000 {
		return common.NewError("plan description is too long (max 2000 characters)")
	}
	if plan.DurationDays < 1 || plan.DurationDays > 3650 {
		return common.NewError("plan duration must be between 1 and 3650 days")
	}
	if plan.TrafficLimitBytes < 0 || plan.DeviceLimit < 0 || plan.YooKassaPriceKopecks < 1 || plan.PlategaPriceKopecks < 1 || plan.StarsPrice < 1 || plan.SortOrder < 0 {
		return common.NewError("plan limits and sort order must be non-negative; all three provider prices must be positive")
	}
	if plan.TrafficLimitBytes%planTrafficUnitBytes != 0 {
		return common.NewError("traffic limit must be expressed in whole gigabytes")
	}
	if plan.Enabled && len(plan.InboundIds) == 0 {
		return common.NewError("plan must include at least one inbound")
	}
	uniqueIDs := make([]int, 0, len(plan.InboundIds))
	for _, id := range plan.InboundIds {
		if id < 1 {
			return common.NewError("inbound ids must be positive")
		}
		if !slices.Contains(uniqueIDs, id) {
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	plan.InboundIds = uniqueIDs
	if len(uniqueIDs) > 0 {
		var count int64
		query := database.GetDB().Model(&model.Inbound{}).Where("user_id = ? AND id IN ?", userID, uniqueIDs)
		if plan.Enabled {
			query = query.Where("enable = ?", true)
		}
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(uniqueIDs)) {
			return common.NewError("one or more selected inbounds are unavailable or do not belong to this panel user")
		}
	}
	return nil
}

func (s *PlanService) Create(userID int, plan *model.Plan) (*model.Plan, error) {
	if err := s.validate(userID, plan); err != nil {
		return nil, err
	}
	plan.UserId = userID
	// Select all fields so an explicitly disabled plan is not replaced by the
	// database default (Enabled has a default of true).
	if err := database.GetDB().Select("*").Create(plan).Error; err != nil {
		return nil, err
	}
	return plan, nil
}

func (s *PlanService) Update(userID, id int, update *model.Plan) (*model.Plan, error) {
	if err := s.validate(userID, update); err != nil {
		return nil, err
	}
	var plan model.Plan
	if err := database.GetDB().Where("user_id = ? AND id = ?", userID, id).First(&plan).Error; err != nil {
		return nil, err
	}
	plan.Name = update.Name
	plan.Description = update.Description
	plan.DurationDays = update.DurationDays
	plan.TrafficLimitBytes = update.TrafficLimitBytes
	plan.DeviceLimit = update.DeviceLimit
	plan.InboundIds = update.InboundIds
	plan.YooKassaPriceKopecks = update.YooKassaPriceKopecks
	plan.PlategaPriceKopecks = update.PlategaPriceKopecks
	plan.StarsPrice = update.StarsPrice
	plan.Enabled = update.Enabled
	plan.SortOrder = update.SortOrder
	if err := database.GetDB().Save(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

// disablePlansForInbound removes the deleted inbound from every matching plan
// and disables those plans in the same transaction as the inbound deletion.
func disablePlansForInbound(tx *gorm.DB, inboundID int) error {
	if !tx.Migrator().HasTable(&model.Plan{}) {
		return nil
	}
	var plans []model.Plan
	if err := tx.Find(&plans).Error; err != nil {
		return err
	}
	for i := range plans {
		plan := &plans[i]
		if !slices.Contains(plan.InboundIds, inboundID) {
			continue
		}
		remaining := make([]int, 0, len(plan.InboundIds)-1)
		for _, id := range plan.InboundIds {
			if id != inboundID {
				remaining = append(remaining, id)
			}
		}
		plan.InboundIds = remaining
		plan.Enabled = false
		if err := tx.Model(plan).Select("InboundIds", "Enabled").Updates(plan).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *PlanService) Delete(userID, id int) error {
	result := database.GetDB().Where("user_id = ? AND id = ?", userID, id).Delete(&model.Plan{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
