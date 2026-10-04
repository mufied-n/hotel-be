package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TwilioSender mengirim pesan WhatsApp melalui Twilio Programmable Messaging REST API.
// ponytail: implementasi murni standard library net/http tanpa mengimpor SDK eksternal twilio-go.
type TwilioSender struct {
	AccountSID string
	AuthToken  string
	FromPhone  string
	BaseURL    string
	HTTPClient *http.Client
	Log        *slog.Logger
}

func NewTwilioSender(cfg WhatsAppConfig, l *slog.Logger) (*TwilioSender, error) {
	if cfg.AccountSID == "" {
		return nil, fmt.Errorf("%w: WHATSAPP_ACCOUNT_SID wajib diisi untuk provider twilio", ErrMissingConfiguration)
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%w: WHATSAPP_API_KEY (Twilio Auth Token) wajib diisi untuk provider twilio", ErrMissingConfiguration)
	}
	if cfg.FromPhone == "" {
		return nil, fmt.Errorf("%w: WHATSAPP_FROM_PHONE wajib diisi untuk provider twilio", ErrMissingConfiguration)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", cfg.AccountSID)
	}
	if l == nil {
		l = slog.Default()
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	from := cfg.FromPhone
	if !strings.HasPrefix(from, "whatsapp:") {
		from = "whatsapp:" + from
	}

	return &TwilioSender{
		AccountSID: cfg.AccountSID,
		AuthToken:  cfg.APIKey,
		FromPhone:  from,
		BaseURL:    baseURL,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
		Log: l,
	}, nil
}

func (s *TwilioSender) SendBookingConfirmation(ctx context.Context, msg WhatsAppBookingMessage) error {
	phoneE164, err := NormalizePhoneE164(msg.ToPhone)
	if err != nil {
		return err
	}

	text := FormatWhatsAppConfirmation(msg)

	to := phoneE164
	if !strings.HasPrefix(to, "whatsapp:") {
		to = "whatsapp:" + to
	}

	data := url.Values{}
	data.Set("From", s.FromPhone)
	data.Set("To", to)
	data.Set("Body", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create twilio request: %w", err)
	}
	req.SetBasicAuth(s.AccountSID, s.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWhatsAppDeliveryFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: twilio returned status %d", ErrWhatsAppDeliveryFailed, resp.StatusCode)
	}

	s.Log.InfoContext(ctx, "whatsapp.sent",
		"provider", "twilio",
		"to", to,
		"ref", msg.Reference,
	)

	return nil
}
