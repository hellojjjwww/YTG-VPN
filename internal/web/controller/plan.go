package controller

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

type PlanController struct {
	planService service.PlanService
}

type planPayload struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	DurationDays      int    `json:"durationDays"`
	TrafficLimitBytes int64  `json:"trafficLimitBytes"`
	DeviceLimit       int    `json:"deviceLimit"`
	InboundIds        []int  `json:"inboundIds"`
	YooKassaPriceKopecks int64 `json:"yooKassaPriceKopecks"`
	PlategaPriceKopecks  int64 `json:"plategaPriceKopecks"`
	StarsPrice           int64 `json:"starsPrice"`
	Enabled              bool  `json:"enabled"`
	SortOrder            int   `json:"sortOrder"`
}

func NewPlanController(g *gin.RouterGroup) *PlanController {
	a := &PlanController{}
	g.GET("", a.list)
	g.POST("", a.create)
	g.POST("/:id", a.update)
	g.DELETE("/:id", a.del)
	return a
}

func (a *PlanController) list(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), fmt.Errorf("panel user is unavailable"))
		return
	}
	plans, err := a.planService.List(user.Id)
	jsonObj(c, plans, err)
}

func (a *PlanController) readPayload(c *gin.Context) (*model.Plan, error) {
	var payload planPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, err
	}
	return &model.Plan{
		Name:              payload.Name,
		Description:       payload.Description,
		DurationDays:      payload.DurationDays,
		TrafficLimitBytes: payload.TrafficLimitBytes,
		DeviceLimit:       payload.DeviceLimit,
		InboundIds:        payload.InboundIds,
		YooKassaPriceKopecks: payload.YooKassaPriceKopecks,
		PlategaPriceKopecks:  payload.PlategaPriceKopecks,
		StarsPrice:           payload.StarsPrice,
		Enabled:              payload.Enabled,
		SortOrder:            payload.SortOrder,
	}, nil
}

func (a *PlanController) create(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), fmt.Errorf("panel user is unavailable"))
		return
	}
	plan, err := a.readPayload(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	created, err := a.planService.Create(user.Id, plan)
	jsonObj(c, created, err)
}

func (a *PlanController) update(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), fmt.Errorf("panel user is unavailable"))
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), fmt.Errorf("invalid plan id %q", c.Param("id")))
		return
	}
	plan, err := a.readPayload(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	updated, err := a.planService.Update(user.Id, id, plan)
	jsonObj(c, updated, err)
}

func (a *PlanController) del(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), fmt.Errorf("panel user is unavailable"))
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), fmt.Errorf("invalid plan id %q", c.Param("id")))
		return
	}
	jsonObj(c, "", a.planService.Delete(user.Id, id))
}
