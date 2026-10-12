package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/tgbot"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

const maxTelegramInitDataAge = 24 * time.Hour

type StoreController struct {
	payments      *service.PaymentService
	settingService service.SettingService
	userService   panel.UserService
}

type storeTelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

func NewStoreController(g *gin.RouterGroup, payments *service.PaymentService) *StoreController {
	a := &StoreController{
		payments:       payments,
		settingService: service.SettingService{},
		userService:    panel.UserService{},
	}
	g.GET("/store", a.page)
	g.GET("/store/", a.page)
	api := g.Group("/store/api")
	api.GET("/catalog", a.catalog)
	api.GET("/me", a.me)
	api.POST("/checkout", a.checkout)
	api.GET("/orders/:uid", a.order)
	g.POST("/store/api/webhooks/platega", a.plategaWebhook)
	g.POST("/store/api/webhooks/yookassa", a.yooKassaWebhook)
	return a
}

func (a *StoreController) page(c *gin.Context) {
	serveDistPage(c, "store.html")
}

func (a *StoreController) authenticate(c *gin.Context) (storeTelegramUser, bool) {
	var user storeTelegramUser
	raw := c.GetHeader("X-Telegram-Init-Data")
	if raw == "" || len(raw) > 8192 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "open this page from Telegram"})
		return user, false
	}
	token, err := a.settingService.GetTgBotToken()
	if err != nil || strings.TrimSpace(token) == "" {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Telegram bot is not configured"})
		return user, false
	}
	values, err := url.ParseQuery(raw)
	if err != nil || values.Get("hash") == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid Telegram session"})
		return user, false
	}
	keys := make([]string, 0, len(values))
	for key, items := range values {
		if len(items) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid Telegram session"})
			return user, false
		}
		if key != "hash" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secretMAC.Write([]byte(token))
	checkMAC := hmac.New(sha256.New, secretMAC.Sum(nil))
	_, _ = checkMAC.Write([]byte(strings.Join(parts, "\n")))
	expected := checkMAC.Sum(nil)
	provided, decodeErr := hex.DecodeString(values.Get("hash"))
	if decodeErr != nil || len(provided) != len(expected) || subtle.ConstantTimeCompare(provided, expected) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid Telegram signature"})
		return user, false
	}
	authDate, dateErr := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	issued := time.Unix(authDate, 0)
	if dateErr != nil || time.Since(issued) > maxTelegramInitDataAge || issued.After(time.Now().Add(10*time.Minute)) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Telegram session expired"})
		return user, false
	}
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil || user.ID < 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Telegram user data is invalid"})
		return user, false
	}
	return user, true
}

func (a *StoreController) owner(c *gin.Context) (*model.User, bool) {
	owner, err := a.userService.GetFirstUser()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "panel account is unavailable"})
		return nil, false
	}
	return owner, true
}

func (a *StoreController) catalog(c *gin.Context) {
	owner, ok := a.owner(c)
	if !ok {
		return
	}
	configuredProviders, err := a.payments.GetProviderConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type publicProvider struct {
		Provider string `json:"provider"`
		Enabled  bool   `json:"enabled"`
	}
	providers := make([]publicProvider, 0, len(configuredProviders))
	for _, provider := range configuredProviders {
		providers = append(providers, publicProvider{Provider: provider.Provider, Enabled: provider.Enabled})
	}
	var plans []model.Plan
	if err := database.GetDB().Where("user_id = ? AND enabled = ?", owner.Id, true).Order("sort_order asc, id asc").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ids := make([]int, 0)
	for _, plan := range plans {
		ids = append(ids, plan.InboundIds...)
	}
	var inbounds []model.Inbound
	if len(ids) > 0 {
		if err := database.GetDB().Where("user_id = ? AND id IN ? AND enable = ?", owner.Id, ids, true).Find(&inbounds).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	byID := make(map[int]model.Inbound, len(inbounds))
	for _, inbound := range inbounds {
		byID[inbound.Id] = inbound
	}
	type location struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Protocol string `json:"protocol"`
		Port     int    `json:"port"`
	}
	type publicPlan struct {
		ID                  int        `json:"id"`
		Name                string     `json:"name"`
		Description         string     `json:"description"`
		DurationDays        int        `json:"durationDays"`
		TrafficLimitBytes   int64      `json:"trafficLimitBytes"`
		DeviceLimit         int        `json:"deviceLimit"`
		YooKassaPriceKopecks int64     `json:"yooKassaPriceKopecks"`
		PlategaPriceKopecks int64      `json:"plategaPriceKopecks"`
		StarsPrice          int64      `json:"starsPrice"`
		Locations           []location `json:"locations"`
	}
	out := make([]publicPlan, 0, len(plans))
	for _, plan := range plans {
		locations := make([]location, 0, len(plan.InboundIds))
		for _, id := range plan.InboundIds {
			if inbound, found := byID[id]; found {
				name := strings.TrimSpace(inbound.Remark)
				if name == "" {
					name = strings.ToUpper(string(inbound.Protocol)) + " · " + strconv.Itoa(inbound.Port)
				}
				locations = append(locations, location{ID: id, Name: name, Protocol: string(inbound.Protocol), Port: inbound.Port})
			}
		}
		if len(locations) != len(plan.InboundIds) {
			continue
		}
		out = append(out, publicPlan{
			ID: plan.Id, Name: plan.Name, Description: plan.Description,
			DurationDays: plan.DurationDays, TrafficLimitBytes: plan.TrafficLimitBytes,
			DeviceLimit: plan.DeviceLimit, YooKassaPriceKopecks: plan.YooKassaPriceKopecks,
			PlategaPriceKopecks: plan.PlategaPriceKopecks, StarsPrice: plan.StarsPrice,
			Locations: locations,
		})
	}
	c.JSON(http.StatusOK, gin.H{"plans": out, "providers": providers})
}

func (a *StoreController) me(c *gin.Context) {
	user, ok := a.authenticate(c)
	if !ok {
		return
	}
	records, err := (&service.ClientService{}).GetRecordsByTgID(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	clients := make([]gin.H, 0, len(records))
	for _, record := range records {
		profileName := strings.TrimSpace(record.Comment)
		if profileName == "" {
			profileName = record.Email
		}
		inboundIDs, err := (&service.ClientService{}).GetInboundIdsForRecord(record.Id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load VPN profile locations"})
			return
		}
		clients = append(clients, gin.H{
			"id": record.Id, "name": profileName, "enabled": record.Enable,
			"expiryTime": record.ExpiryTime, "totalGB": record.TotalGB,
			"deviceLimit": record.LimitHwid, "subId": record.SubID,
			"subscriptionUrl": a.subscriptionURL(record.SubID),
			"inboundIds": inboundIDs,
		})
	}
	var orderRows []model.PaymentOrder
	if err := database.GetDB().Where("tg_id = ?", user.ID).Order("created_at desc").Limit(30).Find(&orderRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	orders := make([]gin.H, 0, len(orderRows))
	for _, order := range orderRows {
		orders = append(orders, gin.H{
			"uid": order.UID, "planName": order.PlanName, "provider": order.Provider,
			"amount": order.Amount, "currency": order.Currency, "status": order.Status,
			"fulfillmentStatus": order.FulfillmentStatus, "createdAt": order.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"user": user, "clients": clients, "orders": orders})
}

func (a *StoreController) subscriptionURL(subID string) string {
	if subID == "" {
		return ""
	}
	base, err := a.settingService.GetSubURI()
	if err == nil && strings.TrimSpace(base) != "" {
		parsed, parseErr := url.Parse(base)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return ""
		}
		return strings.TrimRight(base, "/") + "/" + url.PathEscape(subID)
	}
	path, _ := a.settingService.GetSubPath()
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	domain, _ := a.settingService.GetSubDomain()
	if domain == "" {
		domain, _ = a.settingService.GetWebDomain()
	}
	if domain == "" {
		return ""
	}
	if strings.Contains(domain, "://") {
		parsed, parseErr := url.Parse(domain)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return ""
		}
		domain = parsed.Host
	}
	if strings.ContainsAny(domain, "\r\n/@?#") {
		return ""
	}
	port, _ := a.settingService.GetSubPort()
	if !strings.Contains(domain, ":") && port > 0 && port != 80 && port != 443 {
		domain += ":" + strconv.Itoa(port)
	}
	return "https://" + domain + path + url.PathEscape(subID)
}

type storeCheckoutRequest struct {
	PlanID    int    `json:"planId"`
	ClientID  int    `json:"clientId"`
	Provider  string `json:"provider"`
}

func (a *StoreController) checkout(c *gin.Context) {
	user, ok := a.authenticate(c)
	if !ok {
		return
	}
	owner, ok := a.owner(c)
	if !ok {
		return
	}
	var input storeCheckoutRequest
	if err := c.ShouldBindJSON(&input); err != nil || input.PlanID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "select a valid plan"})
		return
	}
	order, err := a.payments.CreateOrder(owner.Id, user.ID, input.PlanID, input.ClientID, input.Provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	returnURL, err := a.publicURL(c, "store?payment=return&order="+url.QueryEscape(order.UID))
	if err != nil {
		_ = a.payments.FailCheckout(order.Id, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var checkoutURL string
	if order.Provider == service.PaymentStars {
		invoiceURL, invoiceErr := tgbot.CreateStarsInvoiceLink(c.Request.Context(), order.PlanName, order.PlanDescription, order.UID, order.Amount)
		if invoiceErr != nil {
			_ = a.payments.FailCheckout(order.Id, invoiceErr)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": invoiceErr.Error()})
			return
		}
		checkoutURL = invoiceURL
		if err := a.payments.SetCheckout(order.Id, order.UID, checkoutURL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save Stars invoice"})
			return
		}
	} else {
		checkout, checkoutErr := a.payments.StartRUBCheckout(c.Request.Context(), order, returnURL)
		if checkoutErr != nil {
			_ = a.payments.FailCheckout(order.Id, checkoutErr)
			c.JSON(http.StatusBadGateway, gin.H{"error": checkoutErr.Error()})
			return
		}
		checkoutURL = checkout.URL
		if err := a.payments.SetCheckout(order.Id, checkout.PaymentID, checkout.URL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "payment was created but could not be saved"})
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"orderUid": order.UID, "provider": order.Provider, "checkoutUrl": checkoutURL, "amount": order.Amount, "currency": order.Currency, "createdAt": order.CreatedAt})
}

func (a *StoreController) publicURL(c *gin.Context, path string) (string, error) {
	domain, _ := a.settingService.GetWebDomain()
	if strings.TrimSpace(domain) == "" {
		return "", errors.New("configure a public panel domain before creating checkout links")
	}
	if strings.Contains(domain, "://") {
		parsed, err := url.Parse(domain)
		if err != nil || parsed.Host == "" {
			return "", errors.New("panel web domain is invalid")
		}
		domain = parsed.Host
	}
	if strings.ContainsAny(domain, "\r\n/@?#") {
		return "", errors.New("panel web domain is invalid")
	}
	basePath := c.GetString("base_path")
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	return "https://" + domain + basePath + strings.TrimLeft(path, "/"), nil
}

func (a *StoreController) order(c *gin.Context) {
	user, ok := a.authenticate(c)
	if !ok {
		return
	}
	order, err := a.payments.GetCustomerOrder(c.Param("uid"), user.ID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}
	if order.Status == service.PaymentPaid && order.FulfillmentStatus != service.FulfillmentComplete {
		_ = a.payments.FulfillOrder(order.UID)
		order, _ = a.payments.GetCustomerOrder(c.Param("uid"), user.ID)
	}
	c.JSON(http.StatusOK, gin.H{"order": gin.H{
		"uid": order.UID, "planName": order.PlanName, "provider": order.Provider,
		"amount": order.Amount, "currency": order.Currency, "status": order.Status,
		"fulfillmentStatus": order.FulfillmentStatus, "createdAt": order.CreatedAt,
	}})
}

func (a *StoreController) plategaWebhook(c *gin.Context) {
	if err := a.payments.HandlePlategaCallback(c.Request.Context(), c.GetHeader("X-MerchantId"), c.GetHeader("X-Secret"), c.Request.Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func (a *StoreController) yooKassaWebhook(c *gin.Context) {
	if err := a.payments.HandleYooKassaWebhook(c.Request.Context(), c.Request.Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func NewPaymentsAdminController(g *gin.RouterGroup, payments *service.PaymentService) {
	g.GET("/config", func(c *gin.Context) {
		configs, err := payments.GetProviderConfigs()
		jsonObj(c, configs, err)
	})
	g.POST("/config", func(c *gin.Context) {
		var request struct {
			Providers []service.PaymentProviderInput `json:"providers"`
		}
		if err := c.ShouldBindJSON(&request); err != nil || len(request.Providers) == 0 {
			jsonObj(c, nil, errors.New("invalid payment provider configuration"))
			return
		}
		configs, err := payments.UpdateProviderConfigs(request.Providers)
		jsonObj(c, configs, err)
	})
	g.GET("/orders", func(c *gin.Context) {
		user := currentPanelUser(c)
		if user == nil {
			jsonObj(c, nil, errors.New("panel user is unavailable"))
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		orders, err := payments.ListOrders(user.Id, limit, offset)
		jsonObj(c, orders, err)
	})
}

func currentPanelUser(c *gin.Context) *model.User {
	return session.GetLoginUser(c)
}
