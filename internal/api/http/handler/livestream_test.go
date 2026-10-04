package handler

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/hotel-booking/internal/api/http/middleware"
	"github.com/example/hotel-booking/internal/guest"
	"github.com/example/hotel-booking/internal/platform/eventbus"
	"github.com/gin-gonic/gin"
)

type mockGuestServiceForLiveStream struct {
	receipt *guest.ReceiptDTO
	err     error
}

func (m *mockGuestServiceForLiveStream) RequestChallenge(ctx context.Context, email string) (int, error) {
	return 60, nil
}
func (m *mockGuestServiceForLiveStream) VerifyChallenge(ctx context.Context, email, code string) (string, *guest.GuestSession, error) {
	return "token", &guest.GuestSession{GuestEmail: email}, nil
}
func (m *mockGuestServiceForLiveStream) ValidateSession(ctx context.Context, rawToken string) (*guest.GuestSession, error) {
	return &guest.GuestSession{GuestEmail: "budi@example.com"}, nil
}
func (m *mockGuestServiceForLiveStream) RevokeSession(ctx context.Context, rawToken string) error {
	return nil
}
func (m *mockGuestServiceForLiveStream) GetSessionProfile(ctx context.Context, session *guest.GuestSession) (*guest.ProfileView, error) {
	return nil, nil
}
func (m *mockGuestServiceForLiveStream) ListBookings(ctx context.Context, email, status string, limit int) ([]guest.BookingSummary, error) {
	return nil, nil
}
func (m *mockGuestServiceForLiveStream) GetBookingDetail(ctx context.Context, email, bookingID string) (*guest.BookingDetail, error) {
	return nil, nil
}
func (m *mockGuestServiceForLiveStream) GetBookingReceipt(ctx context.Context, email, bookingID string) (*guest.ReceiptDTO, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.receipt, nil
}
func (m *mockGuestServiceForLiveStream) GenerateCalendarICS(receipt *guest.ReceiptDTO) ([]byte, error) {
	return []byte("ics"), nil
}

func TestGuestBookingLiveStatus_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		withSess   bool
		guestErr   error
		wantStatus int
		expectSSE  bool
	}{
		{
			name:       "Missing guest session returns 401 Unauthorized",
			withSess:   false,
			wantStatus: http.StatusUnauthorized,
			expectSSE:  false,
		},
		{
			name:       "Booking not found / IDOR violation returns 404",
			withSess:   true,
			guestErr:   guest.ErrBookingNotFound,
			wantStatus: http.StatusNotFound,
			expectSSE:  false,
		},
		{
			name:       "Valid ownership opens SSE stream and receives connected event",
			withSess:   true,
			guestErr:   nil,
			wantStatus: http.StatusOK,
			expectSSE:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := eventbus.NewMemoryBus()
			defer bus.Close()

			deps := Deps{
				GuestSvc: &mockGuestServiceForLiveStream{
					receipt: &guest.ReceiptDTO{
						BookingID: "b-100",
					},
					err: tt.guestErr,
				},
				EventBus: bus,
			}

			r := gin.New()
			r.GET("/live-status/:id", func(c *gin.Context) {
				if tt.withSess {
					c.Request = c.Request.WithContext(middleware.ContextWithGuestSession(c.Request.Context(), &guest.GuestSession{
						GuestEmail: "budi@example.com",
					}))
				}
				GuestBookingLiveStatus(deps)(c)
			})

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			req := httptest.NewRequest(http.MethodGet, "/live-status/b-100", nil).WithContext(ctx)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.expectSSE {
				contentType := w.Header().Get("Content-Type")
				if !strings.Contains(contentType, "text/event-stream") {
					t.Errorf("expected Content-Type text/event-stream, got %s", contentType)
				}

				body := w.Body.String()
				if !strings.Contains(body, "event:connected") && !strings.Contains(body, "event: connected") {
					t.Errorf("expected initial connected event, got:\n%s", body)
				}
			}
		})
	}
}

func TestFrontDeskLiveStream_TableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		busNil     bool
		wantStatus int
	}{
		{
			name:       "Nil event bus returns 503 Service Unavailable",
			busNil:     true,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "Active event bus connects SSE successfully",
			busNil:     false,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bus eventbus.Bus
			if !tt.busNil {
				m := eventbus.NewMemoryBus()
				defer m.Close()
				bus = m
			}

			deps := Deps{
				EventBus: bus,
			}

			r := gin.New()
			r.GET("/front-desk/live-stream", FrontDeskLiveStream(deps))

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			req := httptest.NewRequest(http.MethodGet, "/front-desk/live-stream", nil).WithContext(ctx)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if !tt.busNil {
				contentType := w.Header().Get("Content-Type")
				if !strings.Contains(contentType, "text/event-stream") {
					t.Errorf("expected text/event-stream, got %s", contentType)
				}
				body := w.Body.String()
				if !strings.Contains(body, "front_desk") {
					t.Errorf("expected role front_desk in initial payload, got %s", body)
				}
			}
		})
	}
}

func TestGuestBookingLiveStatus_ReceivesPushedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	bus := eventbus.NewMemoryBus()
	defer bus.Close()

	deps := Deps{
		GuestSvc: &mockGuestServiceForLiveStream{
			receipt: &guest.ReceiptDTO{
				BookingID: "b-live-99",
			},
		},
		EventBus: bus,
	}

	r := gin.New()
	r.GET("/live-status/:id", func(c *gin.Context) {
		c.Request = c.Request.WithContext(middleware.ContextWithGuestSession(c.Request.Context(), &guest.GuestSession{
			GuestEmail: "budi@example.com",
		}))
		GuestBookingLiveStatus(deps)(c)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// In background, publish an event after 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = bus.Publish(context.Background(), "hospitality.booking.b-live-99.confirmed", "payment_confirmed", "evt-99", []byte(`{"status":"CONFIRMED"}`))
	}()

	req := httptest.NewRequest(http.MethodGet, "/live-status/b-live-99", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	reader := bufio.NewReader(w.Body)
	foundPaymentConfirmed := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.Contains(line, "event:payment_confirmed") || strings.Contains(line, "event: payment_confirmed") {
			foundPaymentConfirmed = true
			break
		}
	}

	if !foundPaymentConfirmed {
		t.Errorf("expected to find event: payment_confirmed in stream output, body was:\n%s", w.Body.String())
	}
}
