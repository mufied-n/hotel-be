package notifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrWhatsAppDeliveryFailed = errors.New("whatsapp: delivery failed")
	ErrInvalidPhoneNumber     = errors.New("whatsapp: invalid recipient phone number")
	ErrUnsupportedProvider    = errors.New("whatsapp: unsupported provider")
	ErrMissingConfiguration   = errors.New("whatsapp: missing required configuration")
)

// WhatsAppBookingMessage memuat parameter konfirmasi reservasi via WhatsApp.
type WhatsAppBookingMessage struct {
	ToPhone    string `json:"to_phone"`
	GuestName  string `json:"guest_name"`
	Reference  string `json:"reference"`
	RoomName   string `json:"room_name"`
	CheckIn    string `json:"check_in"`
	CheckOut   string `json:"check_out"`
	VoucherURL string `json:"voucher_url"`
}

// WhatsAppConfig menyimpan konfigurasi multi-provider pengiriman WhatsApp.
type WhatsAppConfig struct {
	Provider      string        // "log", "generic_http", "twilio", "meta_cloud"
	BaseURL       string        // Custom Base URL jika ada
	APIKey        string        // Auth Token / Secret / Bearer Token
	AccountSID    string        // Twilio Account SID
	PhoneNumberID string        // Meta Cloud Phone Number ID
	FromPhone     string        // Nomor pengirim Twilio / WhatsApp sender
	Timeout       time.Duration // Timeout HTTP request
}

// WhatsAppSender antarmuka pengiriman pesan WhatsApp.
type WhatsAppSender interface {
	SendBookingConfirmation(ctx context.Context, msg WhatsAppBookingMessage) error
}

// NewWhatsAppSender adalah factory modular untuk membuat instans WhatsAppSender sesuai konfigurasi.
// ponytail: satu factory ringkas menggantikan kebutuhan dynamic plugin loader yang berlebihan.
func NewWhatsAppSender(cfg WhatsAppConfig, log *slog.Logger) (WhatsAppSender, error) {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "log", "mock":
		return NewLogWhatsApp(log), nil
	case "generic_http", "gateway", "fonnte", "wablas", "waha", "qontak":
		return NewGenericGatewaySender(cfg, log)
	case "twilio":
		return NewTwilioSender(cfg, log)
	case "meta_cloud", "waba", "meta":
		return NewMetaCloudSender(cfg, log)
	default:
		return nil, fmt.Errorf("%w: '%s'. Didukung: 'log', 'generic_http', 'twilio', 'meta_cloud'", ErrUnsupportedProvider, cfg.Provider)
	}
}

// LogWhatsAppSender adalah implementasi pencatatan structured log & in-memory history untuk testing/development.
type LogWhatsAppSender struct {
	Log     *slog.Logger
	mu      sync.RWMutex
	History []WhatsAppBookingMessage
}

func NewLogWhatsApp(l *slog.Logger) *LogWhatsAppSender {
	if l == nil {
		l = slog.Default()
	}
	return &LogWhatsAppSender{
		Log:     l,
		History: make([]WhatsAppBookingMessage, 0),
	}
}

func (s *LogWhatsAppSender) SendBookingConfirmation(ctx context.Context, msg WhatsAppBookingMessage) error {
	phone, err := NormalizePhoneDigitsOnly(msg.ToPhone)
	if err != nil {
		return err
	}

	text := FormatWhatsAppConfirmation(msg)

	s.Log.InfoContext(ctx, "whatsapp.booking_confirmed",
		"provider", "log",
		"to", phone,
		"guest", msg.GuestName,
		"ref", msg.Reference,
		"text", text,
	)

	s.mu.Lock()
	s.History = append(s.History, msg)
	s.mu.Unlock()

	return nil
}

// NormalizePhoneDigitsOnly mengekstrak digit angka dan menormalisasi awalan lokal Indonesia ke kode negara 62.
func NormalizePhoneDigitsOnly(phone string) (string, error) {
	var sb strings.Builder
	for _, r := range phone {
		if unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	digits := sb.String()
	if len(digits) < 7 {
		return "", ErrInvalidPhoneNumber
	}

	// Normalisasi format Indonesia: 08xx -> 628xx
	if strings.HasPrefix(digits, "0") {
		digits = "62" + digits[1:]
	}
	return digits, nil
}

// NormalizePhoneE164 menghasilkan format standar internasional E.164 (+628xxx).
func NormalizePhoneE164(phone string) (string, error) {
	digits, err := NormalizePhoneDigitsOnly(phone)
	if err != nil {
		return "", err
	}
	return "+" + digits, nil
}

// FormatWhatsAppConfirmation menghasilkan teks bahasa Indonesia yang ramah dan formal khas perhotelan Yogyakarta.
func FormatWhatsAppConfirmation(msg WhatsAppBookingMessage) string {
	return fmt.Sprintf(
		"Halo Bapak/Ibu %s,\n\n"+
			"Terima kasih telah memilih *Pulang ke Uttara (Yogyakarta)*.\n"+
			"Pemesanan Anda telah *TERKONFIRMASI*:\n\n"+
			"• *Kode Reservasi:* %s\n"+
			"• *Tipe Kamar:* %s\n"+
			"• *Check-in:* %s (mulai 14.00 WIB)\n"+
			"• *Check-out:* %s (maks 12.00 WIB)\n\n"+
			"Unduh e-voucher resmi Anda untuk kemudahan check-in di meja depan:\n%s\n\n"+
			"Kami menantikan kedatangan Anda di Jl. Kaliurang Km 5, Sleman. Sampai jumpa di Yogyakarta!",
		msg.GuestName, msg.Reference, msg.RoomName, msg.CheckIn, msg.CheckOut, msg.VoucherURL,
	)
}
