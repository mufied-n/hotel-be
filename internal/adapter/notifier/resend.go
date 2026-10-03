package notifier

import (
	"bytes"
	"context"
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
	// ErrEmailDispatchFailed menandakan error saat memanggil REST API Resend.
	ErrEmailDispatchFailed = errors.New("resend: email dispatch failed")
)

// ResendNotifier mengimplementasikan booking.Notifier menggunakan Resend REST API (https://api.resend.com).
// Menggunakan header Idempotency-Key untuk menjamin deduplikasi pengiriman email (UU PDP No. 27/2022).
type ResendNotifier struct {
	BaseURL   string
	APIKey    string
	FromEmail string
	Client    *http.Client
	Log       *slog.Logger
}

// NewResend membuat instance baru ResendNotifier.
func NewResend(baseURL, apiKey, fromEmail string, log *slog.Logger) *ResendNotifier {
	if baseURL == "" {
		baseURL = "https://api.resend.com"
	}
	if fromEmail == "" {
		fromEmail = "Pulang ke Uttara <reservations@pulangkeuttara.com>"
	}
	if log == nil {
		log = slog.Default()
	}
	return &ResendNotifier{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		APIKey:    apiKey,
		FromEmail: fromEmail,
		Client:    &http.Client{Timeout: 10 * time.Second},
		Log:       log,
	}
}

type sendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

type sendEmailResponse struct {
	ID      string `json:"id"`
	Message string `json:"message,omitempty"`
}

// SendBookingConfirmed menyusun template konfirmasi menginap dan mengirimkannya via Resend.
func (n *ResendNotifier) SendBookingConfirmed(ctx context.Context, b booking.Booking) error {
	if n.APIKey == "" {
		return errors.New("resend: API key is not configured")
	}

	htmlBody := renderBookingConfirmationHTML(b)

	reqBody := sendEmailRequest{
		From:    n.FromEmail,
		To:      []string{b.GuestEmail},
		Subject: fmt.Sprintf("Konfirmasi Reservasi Hotel Pulang ke Uttara - %s", b.ID),
		HTML:    htmlBody,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("resend: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/emails", n.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("resend: new request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+n.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	// Idempotency-Key menjamin Resend tidak mengirim email ganda saat worker outbox retry
	httpReq.Header.Set("Idempotency-Key", fmt.Sprintf("email-confirmed-%s", b.ID))

	resp, err := n.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("resend: do request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("resend: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		n.Log.ErrorContext(ctx, "resend.send_email_failed", "status", resp.StatusCode, "body", string(respBytes))
		return fmt.Errorf("%w: status %d: %s", ErrEmailDispatchFailed, resp.StatusCode, string(respBytes))
	}

	var res sendEmailResponse
	_ = json.Unmarshal(respBytes, &res)

	n.Log.InfoContext(ctx, "resend.email_sent",
		"booking_id", b.ID,
		"resend_id", res.ID,
		"recipient", b.GuestEmail,
	)
	return nil
}

// renderBookingConfirmationHTML menghasilkan template email responsif bermerek Pulang ke Uttara (Yogyakarta).
func renderBookingConfirmationHTML(b booking.Booking) string {
	checkInStr := b.CheckIn.Format("02 Jan 2006")
	checkOutStr := b.CheckOut.Format("02 Jan 2006")

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background-color: #F7F5F0; margin: 0; padding: 24px; color: #2D2B2A; }
    .card { max-width: 600px; margin: 0 auto; background: #FFFFFF; border-radius: 8px; border: 1px solid #E6E2D8; padding: 32px; box-shadow: 0 4px 12px rgba(0,0,0,0.05); }
    .header { text-align: center; border-bottom: 2px solid #9B4A2C; padding-bottom: 20px; margin-bottom: 24px; }
    .header h1 { margin: 0; color: #2D2B2A; font-size: 24px; letter-spacing: 1px; }
    .header p { margin: 4px 0 0; color: #9B4A2C; font-size: 14px; text-transform: uppercase; font-weight: bold; }
    .section-title { font-size: 16px; font-weight: 600; color: #2D2B2A; margin-top: 24px; margin-bottom: 12px; }
    .info-table { width: 100%%; border-collapse: collapse; margin-bottom: 20px; }
    .info-table td { padding: 8px 0; font-size: 14px; }
    .info-table td.label { color: #666; width: 40%%; }
    .info-table td.val { font-weight: 500; color: #111; }
    .badge { display: inline-block; padding: 4px 12px; background: #E6F4EA; color: #137333; font-weight: bold; border-radius: 16px; font-size: 12px; }
    .footer { text-align: center; font-size: 12px; color: #888; border-top: 1px solid #E6E2D8; padding-top: 20px; margin-top: 32px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <h1>PULANG KE UTTARA</h1>
      <p>Konfirmasi Reservasi Resmi</p>
    </div>
    <p>Halo <strong>%s</strong>,</p>
    <p>Terima kasih telah memilih Pulang ke Uttara. Reservasi Anda telah <strong>berhasil dikonfirmasi dan lunas</strong>. Kami menantikan kehadiran Anda di Yogyakarta.</p>
    
    <div class="section-title">Detail Reservasi</div>
    <table class="info-table">
      <tr><td class="label">Nomor Booking</td><td class="val"><code>%s</code></td></tr>
      <tr><td class="label">Status</td><td class="val"><span class="badge">CONFIRMED</span></td></tr>
      <tr><td class="label">Check-in</td><td class="val">%s (Mulai 15:00 WIB)</td></tr>
      <tr><td class="label">Check-out</td><td class="val">%s (Hingga 12:00 WIB)</td></tr>
      <tr><td class="label">Jumlah Kamar</td><td class="val">%d Kamar</td></tr>
      <tr><td class="label">Total Pembayaran</td><td class="val"><strong>Rp %d %s</strong> (Termasuk Pajak PB1 10%%)</td></tr>
    </table>

    <div class="footer">
      <p>Jl. Kaliurang KM 5.5 No. 12, Sleman, D.I. Yogyakarta</p>
      <p>Email: reservations@pulangkeuttara.com | Telp: +62 274 555-0199</p>
    </div>
  </div>
</body>
</html>`,
		b.GuestName,
		b.ID,
		checkInStr,
		checkOutStr,
		b.NumRooms,
		b.TotalPriceMinor,
		b.Currency,
	)
}

var _ booking.Notifier = (*ResendNotifier)(nil)
