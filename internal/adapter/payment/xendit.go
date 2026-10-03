package payment

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/hotel-booking/internal/booking"
)

var (
	// ErrInvalidWebhookToken menandakan token callback Xendit tidak cocok (OWASP Top 10).
	ErrInvalidWebhookToken = errors.New("xendit: invalid webhook token")
	// ErrInvoiceCreationFailed menandakan kegagalan saat membuat invoice ke API Xendit.
	ErrInvoiceCreationFailed = errors.New("xendit: invoice creation failed")
)

// XenditGateway mengimplementasikan booking.PaymentGateway menggunakan Xendit Invoice v2 API.
// Kepatuhan PCI-DSS v4.0 SAQ A: Tamu dialihkan ke hosted checkout Xendit;
// data kartu kredit/sensitif tidak pernah masuk ke server hotel.
type XenditGateway struct {
	BaseURL      string
	SecretKey    string
	WebhookToken string
	AppBaseURL   string
	Client       *http.Client
	Log          *slog.Logger
}

// NewXendit membuat instance baru XenditGateway.
func NewXendit(baseURL, secretKey, webhookToken, appBaseURL string, log *slog.Logger) *XenditGateway {
	if baseURL == "" {
		baseURL = "https://api.xendit.co"
	}
	if appBaseURL == "" {
		appBaseURL = "http://localhost:3000"
	}
	if log == nil {
		log = slog.Default()
	}
	return &XenditGateway{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		SecretKey:    secretKey,
		WebhookToken: webhookToken,
		AppBaseURL:   strings.TrimRight(appBaseURL, "/"),
		Client:       &http.Client{Timeout: 10 * time.Second},
		Log:          log,
	}
}

type createInvoiceRequest struct {
	ExternalID         string `json:"external_id"`
	Amount             int64  `json:"amount"`
	PayerEmail         string `json:"payer_email,omitempty"`
	Description        string `json:"description"`
	InvoiceDuration    int    `json:"invoice_duration"`
	Currency           string `json:"currency"`
	SuccessRedirectURL string `json:"success_redirect_url,omitempty"`
	FailureRedirectURL string `json:"failure_redirect_url,omitempty"`
}

type createInvoiceResponse struct {
	ID         string `json:"id"`
	ExternalID string `json:"external_id"`
	InvoiceURL string `json:"invoice_url"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
}

// XenditWebhookPayload memuat struktur callback webhook yang dikirimkan Xendit saat status pembayaran berubah.
type XenditWebhookPayload struct {
	ID            string `json:"id"`
	ExternalID    string `json:"external_id"`
	Status        string `json:"status"` // "PAID", "SETTLED", "EXPIRED"
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method,omitempty"`
	PaidAt        string `json:"paid_at,omitempty"`
}

// CreateCharge membuat invoice di Xendit dan mengembalikan URL checkout untuk diarahkan ke tamu.
func (g *XenditGateway) CreateCharge(ctx context.Context, b booking.Booking, amountMinor int64, currency string) (booking.ChargeResult, error) {
	durationSec := 1800 // default 30 menit
	if b.ExpiresAt != nil {
		sec := int(time.Until(*b.ExpiresAt).Seconds())
		if sec > 60 {
			durationSec = sec
		}
	}

	reqBody := createInvoiceRequest{
		ExternalID:         b.ID,
		Amount:             amountMinor,
		PayerEmail:         b.GuestEmail,
		Description:        fmt.Sprintf("Hotel Pulang ke Uttara - Reservasi %s", b.ID),
		InvoiceDuration:    durationSec,
		Currency:           currency,
		SuccessRedirectURL: fmt.Sprintf("%s/booking/status/%s?payment=success", g.AppBaseURL, b.ID),
		FailureRedirectURL: fmt.Sprintf("%s/booking/status/%s?payment=failed", g.AppBaseURL, b.ID),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return booking.ChargeResult{}, fmt.Errorf("xendit: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v2/invoices", g.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return booking.ChargeResult{}, fmt.Errorf("xendit: new request: %w", err)
	}

	// Basic Auth: Secret key sebagai username, password kosong
	auth := base64.StdEncoding.EncodeToString([]byte(g.SecretKey + ":"))
	httpReq.Header.Set("Authorization", "Basic "+auth)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.Client.Do(httpReq)
	if err != nil {
		return booking.ChargeResult{}, fmt.Errorf("xendit: do request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return booking.ChargeResult{}, fmt.Errorf("xendit: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		g.Log.ErrorContext(ctx, "xendit.create_invoice_failed", "status", resp.StatusCode, "body", string(respBytes))
		return booking.ChargeResult{}, fmt.Errorf("%w: status %d: %s", ErrInvoiceCreationFailed, resp.StatusCode, string(respBytes))
	}

	var res createInvoiceResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return booking.ChargeResult{}, fmt.Errorf("xendit: unmarshal response: %w", err)
	}

	if res.InvoiceURL == "" {
		return booking.ChargeResult{}, fmt.Errorf("%w: missing invoice_url in response", ErrInvoiceCreationFailed)
	}

	return booking.ChargeResult{
		PaymentURL: res.InvoiceURL,
		Reference:  res.ID,
	}, nil
}

// VerifyWebhook memvalidasi header x-callback-token secara constant-time (anti timing-attack)
// dan melakukan unmarshal payload XenditWebhookPayload.
func (g *XenditGateway) VerifyWebhook(headerToken string, body []byte) (XenditWebhookPayload, error) {
	if g.WebhookToken == "" {
		return XenditWebhookPayload{}, errors.New("xendit: webhook token is not configured")
	}

	// Constant-time compare mencegah timing attack (OWASP Top 10)
	if subtle.ConstantTimeCompare([]byte(headerToken), []byte(g.WebhookToken)) != 1 {
		return XenditWebhookPayload{}, ErrInvalidWebhookToken
	}

	var payload XenditWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return XenditWebhookPayload{}, fmt.Errorf("xendit: parse webhook payload: %w", err)
	}

	if payload.ExternalID == "" {
		return XenditWebhookPayload{}, errors.New("xendit: webhook payload missing external_id")
	}

	return payload, nil
}

var _ booking.PaymentGateway = (*XenditGateway)(nil)
