package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// GenericGatewaySender mengirim pesan WhatsApp melalui REST gateway eksternal pihak ketiga (Fonnte / Wablas / Waha / Qontak).
type GenericGatewaySender struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Log        *slog.Logger
}

func NewGenericGatewaySender(cfg WhatsAppConfig, l *slog.Logger) (*GenericGatewaySender, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%w: WHATSAPP_API_KEY wajib diisi untuk provider generic_http", ErrMissingConfiguration)
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.fonnte.com/send"
	}
	if l == nil {
		l = slog.Default()
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &GenericGatewaySender{
		BaseURL: baseURL,
		APIKey:  cfg.APIKey,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
		Log: l,
	}, nil
}

// HTTPWhatsAppSender adalah alias kompatibilitas untuk GenericGatewaySender.
type HTTPWhatsAppSender = GenericGatewaySender

// NewHTTPWhatsApp adalah pembungkus konstruktor untuk backwards-compatibility.
func NewHTTPWhatsApp(baseURL, apiKey string, l *slog.Logger) *HTTPWhatsAppSender {
	s, _ := NewGenericGatewaySender(WhatsAppConfig{
		Provider: "generic_http",
		BaseURL:  baseURL,
		APIKey:   apiKey,
	}, l)
	return s
}

func (s *GenericGatewaySender) SendBookingConfirmation(ctx context.Context, msg WhatsAppBookingMessage) error {
	phone, err := NormalizePhoneDigitsOnly(msg.ToPhone)
	if err != nil {
		return err
	}

	text := FormatWhatsAppConfirmation(msg)

	payload := map[string]string{
		"target":  phone,
		"message": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}
	req.Header.Set("Authorization", s.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWhatsAppDeliveryFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("%w: gateway returned status %d", ErrWhatsAppDeliveryFailed, resp.StatusCode)
	}

	s.Log.InfoContext(ctx, "whatsapp.sent",
		"provider", "generic_http",
		"to", phone,
		"ref", msg.Reference,
	)

	return nil
}
