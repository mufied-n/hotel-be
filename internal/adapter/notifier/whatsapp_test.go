package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPhoneNormalization(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantDigits string
		wantE164   string
		wantErr    bool
	}{
		{
			name:       "Local 08 format",
			input:      "081234567890",
			wantDigits: "6281234567890",
			wantE164:   "+6281234567890",
			wantErr:    false,
		},
		{
			name:       "International +62 format with dashes and spaces",
			input:      "+62 812-3456-7890",
			wantDigits: "6281234567890",
			wantE164:   "+6281234567890",
			wantErr:    false,
		},
		{
			name:       "Already 62 format",
			input:      "6281234567890",
			wantDigits: "6281234567890",
			wantE164:   "+6281234567890",
			wantErr:    false,
		},
		{
			name:    "Too short phone number",
			input:   "12345",
			wantErr: true,
		},
		{
			name:    "Empty phone number",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digits, err := NormalizePhoneDigitsOnly(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizePhoneDigitsOnly(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && digits != tt.wantDigits {
				t.Errorf("NormalizePhoneDigitsOnly(%q) = %q, want %q", tt.input, digits, tt.wantDigits)
			}

			e164, err := NormalizePhoneE164(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizePhoneE164(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && e164 != tt.wantE164 {
				t.Errorf("NormalizePhoneE164(%q) = %q, want %q", tt.input, e164, tt.wantE164)
			}
		})
	}
}

func TestWhatsAppSender_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		msg       WhatsAppBookingMessage
		wantErr   error
		wantLog   bool
		checkBody func(t *testing.T, text string)
	}{
		{
			name: "Successful Indonesian WhatsApp confirmation",
			msg: WhatsAppBookingMessage{
				ToPhone:    "+6281234567890",
				GuestName:  "Budi Santoso",
				Reference:  "PKU-88219",
				RoomName:   "Superior King Bay Window",
				CheckIn:    "2026-10-10",
				CheckOut:   "2026-10-12",
				VoucherURL: "https://pulang.id/v/PKU-88219",
			},
			wantErr: nil,
			wantLog: true,
			checkBody: func(t *testing.T, text string) {
				if !strings.Contains(text, "Budi Santoso") {
					t.Errorf("expected text to contain guest name Budi Santoso")
				}
				if !strings.Contains(text, "PKU-88219") {
					t.Errorf("expected text to contain reference PKU-88219")
				}
				if !strings.Contains(text, "https://pulang.id/v/PKU-88219") {
					t.Errorf("expected text to contain voucher link")
				}
			},
		},
		{
			name: "Empty recipient phone number returns ErrInvalidPhoneNumber",
			msg: WhatsAppBookingMessage{
				ToPhone:   "",
				GuestName: "Anonim",
			},
			wantErr: ErrInvalidPhoneNumber,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := NewLogWhatsApp(nil)
			ctx := context.Background()
			err := sender.SendBookingConfirmation(ctx, tt.msg)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(sender.History) != 1 {
					t.Fatalf("expected 1 history item, got %d", len(sender.History))
				}
				if tt.checkBody != nil {
					text := FormatWhatsAppConfirmation(tt.msg)
					tt.checkBody(t, text)
				}
			}
		})
	}
}

func TestGenericGatewaySender_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		apiKey     string
		serverCode int
		msg        WhatsAppBookingMessage
		wantErr    bool
	}{
		{
			name:       "Gateway returns 200 OK",
			apiKey:     "test-key",
			serverCode: http.StatusOK,
			msg: WhatsAppBookingMessage{
				ToPhone:   "+62811112222",
				GuestName: "Siti",
			},
			wantErr: false,
		},
		{
			name:       "Gateway returns 400 Bad Request",
			apiKey:     "test-key",
			serverCode: http.StatusBadRequest,
			msg: WhatsAppBookingMessage{
				ToPhone:   "+62811112222",
				GuestName: "Siti",
			},
			wantErr: true,
		},
		{
			name:       "Empty phone returns ErrInvalidPhoneNumber",
			apiKey:     "test-key",
			serverCode: http.StatusOK,
			msg: WhatsAppBookingMessage{
				ToPhone: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != tt.apiKey {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(tt.serverCode)
			}))
			defer server.Close()

			sender := NewHTTPWhatsApp(server.URL, tt.apiKey, nil)
			err := sender.SendBookingConfirmation(context.Background(), tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("SendBookingConfirmation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// Missing API Key validation
	_, err := NewGenericGatewaySender(WhatsAppConfig{Provider: "generic_http"}, nil)
	if !errors.Is(err, ErrMissingConfiguration) {
		t.Errorf("expected ErrMissingConfiguration, got %v", err)
	}
}

func TestTwilioSender_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		cfg        WhatsAppConfig
		serverCode int
		msg        WhatsAppBookingMessage
		wantErr    bool
		checkReq   func(t *testing.T, r *http.Request)
	}{
		{
			name: "Twilio returns 201 Created",
			cfg: WhatsAppConfig{
				AccountSID: "AC_TEST_123",
				APIKey:     "AUTH_TOKEN_456",
				FromPhone:  "+14155238886",
			},
			serverCode: http.StatusCreated,
			msg: WhatsAppBookingMessage{
				ToPhone:   "081234567890",
				GuestName: "Budi",
				Reference: "PKU-TWILIO",
			},
			wantErr: false,
			checkReq: func(t *testing.T, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if !ok || user != "AC_TEST_123" || pass != "AUTH_TOKEN_456" {
					t.Errorf("basic auth mismatch: user=%s, pass=%s", user, pass)
				}
				body, _ := io.ReadAll(r.Body)
				values, _ := url.ParseQuery(string(body))
				if values.Get("From") != "whatsapp:+14155238886" {
					t.Errorf("unexpected From: %s", values.Get("From"))
				}
				if values.Get("To") != "whatsapp:+6281234567890" {
					t.Errorf("unexpected To: %s", values.Get("To"))
				}
				if !strings.Contains(values.Get("Body"), "Budi") {
					t.Errorf("expected Body to contain guest name Budi")
				}
			},
		},
		{
			name: "Twilio returns 400 Bad Request",
			cfg: WhatsAppConfig{
				AccountSID: "AC_TEST_123",
				APIKey:     "AUTH_TOKEN_456",
				FromPhone:  "+14155238886",
			},
			serverCode: http.StatusBadRequest,
			msg: WhatsAppBookingMessage{
				ToPhone:   "081234567890",
				GuestName: "Budi",
			},
			wantErr: true,
		},
		{
			name: "Invalid phone number returns error",
			cfg: WhatsAppConfig{
				AccountSID: "AC_TEST_123",
				APIKey:     "AUTH_TOKEN_456",
				FromPhone:  "+14155238886",
			},
			serverCode: http.StatusCreated,
			msg: WhatsAppBookingMessage{
				ToPhone: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.checkReq != nil {
					tt.checkReq(t, r)
				}
				w.WriteHeader(tt.serverCode)
			}))
			defer server.Close()

			cfg := tt.cfg
			cfg.BaseURL = server.URL
			sender, err := NewTwilioSender(cfg, nil)
			if err != nil {
				t.Fatalf("unexpected NewTwilioSender error: %v", err)
			}

			err = sender.SendBookingConfirmation(context.Background(), tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("SendBookingConfirmation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// Missing configuration validation
	_, err := NewTwilioSender(WhatsAppConfig{}, nil)
	if !errors.Is(err, ErrMissingConfiguration) {
		t.Errorf("expected ErrMissingConfiguration for empty AccountSID, got %v", err)
	}
	_, err = NewTwilioSender(WhatsAppConfig{AccountSID: "AC123"}, nil)
	if !errors.Is(err, ErrMissingConfiguration) {
		t.Errorf("expected ErrMissingConfiguration for empty APIKey, got %v", err)
	}
	_, err = NewTwilioSender(WhatsAppConfig{AccountSID: "AC123", APIKey: "key"}, nil)
	if !errors.Is(err, ErrMissingConfiguration) {
		t.Errorf("expected ErrMissingConfiguration for empty FromPhone, got %v", err)
	}
}

func TestMetaCloudSender_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		cfg        WhatsAppConfig
		serverCode int
		msg        WhatsAppBookingMessage
		wantErr    bool
		checkReq   func(t *testing.T, r *http.Request)
	}{
		{
			name: "Meta Cloud returns 200 OK",
			cfg: WhatsAppConfig{
				PhoneNumberID: "10099887766",
				APIKey:        "META_SYSTEM_TOKEN_999",
			},
			serverCode: http.StatusOK,
			msg: WhatsAppBookingMessage{
				ToPhone:   "081999888777",
				GuestName: "Rina",
				Reference: "PKU-META-01",
			},
			wantErr: false,
			checkReq: func(t *testing.T, r *http.Request) {
				auth := r.Header.Get("Authorization")
				if auth != "Bearer META_SYSTEM_TOKEN_999" {
					t.Errorf("unexpected Authorization header: %s", auth)
				}
				var payload metaCloudPayload
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("failed to decode json body: %v", err)
				}
				if payload.MessagingProduct != "whatsapp" || payload.RecipientType != "individual" {
					t.Errorf("unexpected meta header payload: %+v", payload)
				}
				if payload.To != "6281999888777" {
					t.Errorf("unexpected To digits: %s", payload.To)
				}
				if !strings.Contains(payload.Text.Body, "Rina") {
					t.Errorf("expected Text Body to contain Rina")
				}
			},
		},
		{
			name: "Meta Cloud returns 401 Unauthorized",
			cfg: WhatsAppConfig{
				PhoneNumberID: "10099887766",
				APIKey:        "META_SYSTEM_TOKEN_999",
			},
			serverCode: http.StatusUnauthorized,
			msg: WhatsAppBookingMessage{
				ToPhone:   "081999888777",
				GuestName: "Rina",
			},
			wantErr: true,
		},
		{
			name: "Invalid phone number returns error",
			cfg: WhatsAppConfig{
				PhoneNumberID: "10099887766",
				APIKey:        "META_SYSTEM_TOKEN_999",
			},
			serverCode: http.StatusOK,
			msg: WhatsAppBookingMessage{
				ToPhone: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.checkReq != nil {
					tt.checkReq(t, r)
				}
				w.WriteHeader(tt.serverCode)
			}))
			defer server.Close()

			cfg := tt.cfg
			cfg.BaseURL = server.URL
			sender, err := NewMetaCloudSender(cfg, nil)
			if err != nil {
				t.Fatalf("unexpected NewMetaCloudSender error: %v", err)
			}

			err = sender.SendBookingConfirmation(context.Background(), tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("SendBookingConfirmation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// Missing configuration validation
	_, err := NewMetaCloudSender(WhatsAppConfig{}, nil)
	if !errors.Is(err, ErrMissingConfiguration) {
		t.Errorf("expected ErrMissingConfiguration for empty PhoneNumberID, got %v", err)
	}
	_, err = NewMetaCloudSender(WhatsAppConfig{PhoneNumberID: "123"}, nil)
	if !errors.Is(err, ErrMissingConfiguration) {
		t.Errorf("expected ErrMissingConfiguration for empty APIKey, got %v", err)
	}
}

func TestNewWhatsAppSender_Factory(t *testing.T) {
	tests := []struct {
		name      string
		cfg       WhatsAppConfig
		wantType  string
		wantErrIs error
	}{
		{
			name:     "Default empty provider is LogWhatsAppSender",
			cfg:      WhatsAppConfig{},
			wantType: "*notifier.LogWhatsAppSender",
		},
		{
			name:     "Provider 'log' is LogWhatsAppSender",
			cfg:      WhatsAppConfig{Provider: "log"},
			wantType: "*notifier.LogWhatsAppSender",
		},
		{
			name:     "Provider 'generic_http' is GenericGatewaySender",
			cfg:      WhatsAppConfig{Provider: "generic_http", APIKey: "test"},
			wantType: "*notifier.GenericGatewaySender",
		},
		{
			name:     "Provider 'gateway' is GenericGatewaySender",
			cfg:      WhatsAppConfig{Provider: "gateway", APIKey: "test"},
			wantType: "*notifier.GenericGatewaySender",
		},
		{
			name: "Provider 'twilio' is TwilioSender",
			cfg: WhatsAppConfig{
				Provider:   "twilio",
				AccountSID: "AC123",
				APIKey:     "TOKEN",
				FromPhone:  "+1415",
			},
			wantType: "*notifier.TwilioSender",
		},
		{
			name: "Provider 'meta_cloud' is MetaCloudSender",
			cfg: WhatsAppConfig{
				Provider:      "meta_cloud",
				PhoneNumberID: "12345",
				APIKey:        "TOKEN",
			},
			wantType: "*notifier.MetaCloudSender",
		},
		{
			name: "Provider 'waba' is MetaCloudSender",
			cfg: WhatsAppConfig{
				Provider:      "waba",
				PhoneNumberID: "12345",
				APIKey:        "TOKEN",
			},
			wantType: "*notifier.MetaCloudSender",
		},
		{
			name:      "Unsupported provider returns ErrUnsupportedProvider",
			cfg:       WhatsAppConfig{Provider: "telegram_bot"},
			wantErrIs: ErrUnsupportedProvider,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewWhatsAppSender(tt.cfg, nil)
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("expected error %v, got %v", tt.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s == nil {
				t.Fatalf("expected non-nil sender")
			}
		})
	}
}
