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

// MetaCloudSender mengirim pesan WhatsApp melalui WhatsApp Business Cloud API resmi (Meta Graph API).
// ponytail: implementasi murni standard library net/http tanpa SDK eksternal Facebook.
type MetaCloudSender struct {
	PhoneNumberID string
	AccessToken   string
	BaseURL       string
	HTTPClient    *http.Client
	Log           *slog.Logger
}

type metaTextMessage struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

type metaCloudPayload struct {
	MessagingProduct string          `json:"messaging_product"`
	RecipientType    string          `json:"recipient_type"`
	To               string          `json:"to"`
	Type             string          `json:"type"`
	Text             metaTextMessage `json:"text"`
}

func NewMetaCloudSender(cfg WhatsAppConfig, l *slog.Logger) (*MetaCloudSender, error) {
	if cfg.PhoneNumberID == "" {
		return nil, fmt.Errorf("%w: WHATSAPP_PHONE_NUMBER_ID wajib diisi untuk provider meta_cloud", ErrMissingConfiguration)
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%w: WHATSAPP_API_KEY (Meta System User Token) wajib diisi untuk provider meta_cloud", ErrMissingConfiguration)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://graph.facebook.com/v20.0/%s/messages", cfg.PhoneNumberID)
	}
	if l == nil {
		l = slog.Default()
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &MetaCloudSender{
		PhoneNumberID: cfg.PhoneNumberID,
		AccessToken:   cfg.APIKey,
		BaseURL:       baseURL,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
		Log: l,
	}, nil
}

func (s *MetaCloudSender) SendBookingConfirmation(ctx context.Context, msg WhatsAppBookingMessage) error {
	phone, err := NormalizePhoneDigitsOnly(msg.ToPhone)
	if err != nil {
		return err
	}

	text := FormatWhatsAppConfirmation(msg)

	payload := metaCloudPayload{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               phone,
		Type:             "text",
		Text: metaTextMessage{
			PreviewURL: true,
			Body:       text,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal meta payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create meta request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWhatsAppDeliveryFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: meta cloud returned status %d", ErrWhatsAppDeliveryFailed, resp.StatusCode)
	}

	s.Log.InfoContext(ctx, "whatsapp.sent",
		"provider", "meta_cloud",
		"to", phone,
		"ref", msg.Reference,
	)

	return nil
}
