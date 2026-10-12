package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

const paymentHTTPTimeout = 15 * time.Second

type CheckoutLink struct {
	PaymentID string
	URL       string
}

func (s *PaymentService) StartRUBCheckout(ctx context.Context, order *model.PaymentOrder, returnURL string) (CheckoutLink, error) {
	if order == nil || (order.Provider != PaymentYooKassa && order.Provider != PaymentPlatega) || order.Currency != "RUB" {
		return CheckoutLink{}, errors.New("invalid RUB payment order")
	}
	cfg, err := s.GetProviderConfig(order.Provider)
	if err != nil {
		return CheckoutLink{}, err
	}
	switch order.Provider {
	case PaymentYooKassa:
		return startYooKassaCheckout(ctx, cfg, order, returnURL)
	case PaymentPlatega:
		return startPlategaCheckout(ctx, cfg, order, returnURL)
	default:
		return CheckoutLink{}, errors.New("unsupported payment provider")
	}
}

func startYooKassaCheckout(ctx context.Context, cfg *model.PaymentProviderConfig, order *model.PaymentOrder, returnURL string) (CheckoutLink, error) {
	requestBody := map[string]any{
		"amount": map[string]string{
			"value":    fmt.Sprintf("%d.%02d", order.Amount/100, order.Amount%100),
			"currency": "RUB",
		},
		"capture": true,
		"confirmation": map[string]string{
			"type":       "redirect",
			"return_url": returnURL,
		},
		"description": "VPN plan: " + order.PlanName,
		"metadata":    map[string]string{"order_uid": order.UID},
	}
	var response struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		Confirmation struct {
			URL string `json:"confirmation_url"`
		} `json:"confirmation"`
	}
	if err := jsonRequest(ctx, http.MethodPost, "https://api.yookassa.ru/v3/payments", requestBody, func(req *http.Request) {
		req.SetBasicAuth(cfg.MerchantID, cfg.Secret)
		req.Header.Set("Idempotence-Key", order.UID)
	}, &response); err != nil {
		return CheckoutLink{}, fmt.Errorf("create YooKassa payment: %w", err)
	}
	if response.ID == "" || response.Confirmation.URL == "" {
		return CheckoutLink{}, errors.New("YooKassa returned an incomplete payment response")
	}
	return CheckoutLink{PaymentID: response.ID, URL: response.Confirmation.URL}, nil
}

func verifyYooKassaPayment(ctx context.Context, cfg *model.PaymentProviderConfig, order *model.PaymentOrder) error {
	var payment struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Amount struct {
			Value    string `json:"value"`
			Currency string `json:"currency"`
		} `json:"amount"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := jsonRequest(ctx, http.MethodGet, "https://api.yookassa.ru/v3/payments/"+url.PathEscape(order.ProviderPaymentID), nil, func(req *http.Request) {
		req.SetBasicAuth(cfg.MerchantID, cfg.Secret)
	}, &payment); err != nil {
		return fmt.Errorf("verify YooKassa payment: %w", err)
	}
	amount, err := strconv.ParseFloat(payment.Amount.Value, 64)
	if err != nil {
		return errors.New("YooKassa returned an invalid payment amount")
	}
	kopecks, ok := rublesToKopecks(amount)
	if !ok || payment.Status != "succeeded" || payment.ID != order.ProviderPaymentID || payment.Amount.Currency != "RUB" || payment.Metadata["order_uid"] != order.UID || kopecks != order.Amount {
		return errors.New("YooKassa payment status, amount, currency or order ID does not match")
	}
	return nil
}

func startPlategaCheckout(ctx context.Context, cfg *model.PaymentProviderConfig, order *model.PaymentOrder, returnURL string) (CheckoutLink, error) {
	amount := float64(order.Amount) / 100
	requestBody := map[string]any{
		"paymentDetails": map[string]any{"amount": amount, "currency": "RUB"},
		"description":    "VPN plan: " + order.PlanName,
		"return":         returnURL,
		"failedUrl":      returnURL,
		"payload":        order.UID,
		"metadata": map[string]string{
			"userId": strconv.FormatInt(order.TgID, 10),
		},
	}
	var response struct {
		TransactionID string `json:"transactionId"`
		Status        string `json:"status"`
		URL           string `json:"url"`
		Redirect       string `json:"redirect"`
	}
	if err := jsonRequest(ctx, http.MethodPost, "https://app.platega.io/v2/transaction/process", requestBody, func(req *http.Request) {
		req.Header.Set("X-MerchantId", cfg.MerchantID)
		req.Header.Set("X-Secret", cfg.Secret)
	}, &response); err != nil {
		return CheckoutLink{}, fmt.Errorf("create Platega payment: %w", err)
	}
	paymentURL := response.URL
	if paymentURL == "" {
		paymentURL = response.Redirect
	}
	if response.TransactionID == "" || paymentURL == "" {
		return CheckoutLink{}, errors.New("Platega returned an incomplete payment response")
	}
	return CheckoutLink{PaymentID: response.TransactionID, URL: paymentURL}, nil
}

type plategaPaymentStatus struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Payload       string `json:"payload"`
	PaymentDetails struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"paymentDetails"`
}

func (s *PaymentService) HandlePlategaCallback(ctx context.Context, merchantID, secret string, body io.Reader) error {
	cfg, err := s.storedProviderConfig(PaymentPlatega)
	if err != nil {
		return err
	}
	if cfg.MerchantID == "" || cfg.Secret == "" {
		return errors.New("Platega credentials are not configured")
	}
	if subtle.ConstantTimeCompare([]byte(merchantID), []byte(cfg.MerchantID)) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(cfg.Secret)) != 1 {
		return errors.New("invalid Platega callback credentials")
	}
	var callback struct {
		ID       string  `json:"id"`
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
		Status   string  `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&callback); err != nil {
		return err
	}
	if callback.ID == "" || callback.Status != "CONFIRMED" || callback.Currency != "RUB" {
		return errors.New("Platega callback does not report a confirmed RUB transaction")
	}
	var order model.PaymentOrder
	if err := mustDB().Where("provider = ? AND provider_payment_id = ?", PaymentPlatega, callback.ID).First(&order).Error; err != nil {
		return err
	}
	kopecks, ok := rublesToKopecks(callback.Amount)
	if !ok || kopecks != order.Amount {
		return errors.New("Platega callback amount does not match the order")
	}
	var confirmed plategaPaymentStatus
	if err := jsonRequest(ctx, http.MethodGet, "https://app.platega.io/transaction/"+url.PathEscape(callback.ID), nil, func(req *http.Request) {
		req.Header.Set("X-MerchantId", cfg.MerchantID)
		req.Header.Set("X-Secret", cfg.Secret)
	}, &confirmed); err != nil {
		return fmt.Errorf("verify Platega transaction: %w", err)
	}
	confirmedAmount, valid := rublesToKopecks(confirmed.PaymentDetails.Amount)
	if confirmed.ID != callback.ID || confirmed.Status != "CONFIRMED" || confirmed.PaymentDetails.Currency != "RUB" || confirmedAmount != order.Amount {
		return errors.New("Platega transaction status or amount does not match the order")
	}
	if confirmed.Payload != "" && confirmed.Payload != order.UID {
		return errors.New("Platega transaction payload does not match the order")
	}
	return s.ConfirmProviderOrder(order.UID, PaymentPlatega, callback.ID, "RUB", order.Amount)
}

func (s *PaymentService) HandleYooKassaWebhook(ctx context.Context, body io.Reader) error {
	var event struct {
		Type   string `json:"event"`
		Object struct {
			ID string `json:"id"`
		} `json:"object"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&event); err != nil {
		return err
	}
	if event.Type != "payment.succeeded" || event.Object.ID == "" {
		return nil
	}
	var order model.PaymentOrder
	if err := mustDB().Where("provider = ? AND provider_payment_id = ?", PaymentYooKassa, event.Object.ID).First(&order).Error; err != nil {
		return err
	}
	cfg, err := s.storedProviderConfig(PaymentYooKassa)
	if err != nil {
		return err
	}
	if cfg.MerchantID == "" || cfg.Secret == "" {
		return errors.New("YooKassa credentials are not configured")
	}
	if err := verifyYooKassaPayment(ctx, cfg, &order); err != nil {
		return err
	}
	return s.ConfirmProviderOrder(order.UID, PaymentYooKassa, event.Object.ID, "RUB", order.Amount)
}

func (s *PaymentService) OrderReturnState(uid string, tgID int64) (string, string, error) {
	order, err := s.GetCustomerOrder(uid, tgID)
	if err != nil {
		return "", "", err
	}
	return order.Status, order.FulfillmentStatus, nil
}

func jsonRequest(ctx context.Context, method, endpoint string, input any, headers func(*http.Request), output any) error {
	var requestBody io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		requestBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if headers != nil {
		headers(req)
	}
	client := &http.Client{Timeout: paymentHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("provider returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if output == nil {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	decoder.UseNumber()
	return decoder.Decode(output)
}

func mustDB() *gorm.DB { return database.GetDB() }
