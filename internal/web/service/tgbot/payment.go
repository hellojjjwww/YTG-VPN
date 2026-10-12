package tgbot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/mymmrac/telego"
)

func CreateStarsInvoiceLink(ctx context.Context, title, description, payload string, amount int64) (string, error) {
	title = truncateRunes(strings.TrimSpace(title), 32)
	description = truncateRunes(strings.TrimSpace(description), 255)
	if title == "" || payload == "" || amount < 1 {
		return "", errors.New("invalid Telegram Stars invoice")
	}
	tgBotMutex.Lock()
	activeBot := bot
	tgBotMutex.Unlock()
	if activeBot == nil {
		return "", errors.New("Telegram bot is not running; enable it in Settings → Telegram")
	}
	link, err := activeBot.CreateInvoiceLink(ctx, &telego.CreateInvoiceLinkParams{
		Title:       title,
		Description: description,
		Payload:     payload,
		Currency:    "XTR",
		Prices: []telego.LabeledPrice{{
			Label:  title,
			Amount: int(amount),
		}},
	})
	if err != nil {
		return "", err
	}
	if link == nil || strings.TrimSpace(*link) == "" {
		return "", errors.New("Telegram returned an empty invoice URL")
	}
	return *link, nil
}

func (t *Tgbot) storeURL() (string, error) {
	domain, err := t.settingService.GetWebDomain()
	if err != nil || strings.TrimSpace(domain) == "" {
		return "", errors.New("public panel domain is not configured")
	}
	domain = strings.TrimSpace(domain)
	if strings.Contains(domain, "://") {
		parsed, parseErr := url.Parse(domain)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return "", errors.New("Mini App needs an HTTPS panel domain")
		}
		domain = parsed.Host
	}
	if strings.ContainsAny(domain, "\r\n/@?#") {
		return "", errors.New("panel domain is invalid")
	}
	basePath, err := t.settingService.GetBasePath()
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	return fmt.Sprintf("https://%s%sstore", domain, basePath), nil
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) > max {
		return string(runes[:max])
	}
	return value
}
