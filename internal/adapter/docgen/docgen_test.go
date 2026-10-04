package docgen

import (
	"bytes"
	"testing"
	"time"
)

func TestQRSigningAndVerification(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		reference string
		bookingID string
		checkIn   string
		tamperTok bool
		wantValid bool
	}{
		{
			name:      "Valid signature with custom secret",
			secret:    "my-test-secret-1234",
			reference: "PKU-20261004-9999",
			bookingID: "01923456-789a-bcde-f012-3456789abcde",
			checkIn:   "2026-10-10",
			tamperTok: false,
			wantValid: true,
		},
		{
			name:      "Valid signature with default secret",
			secret:    "",
			reference: "PKU-20261004-8888",
			bookingID: "01923456-789a-bcde-f012-3456789abcdd",
			checkIn:   "2026-10-15",
			tamperTok: false,
			wantValid: true,
		},
		{
			name:      "Tampered token fails verification",
			secret:    "secret-abc",
			reference: "PKU-20261004-7777",
			bookingID: "01923456-789a-bcde-f012-3456789abcdc",
			checkIn:   "2026-10-20",
			tamperTok: true,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := SignVoucherToken(tt.secret, tt.reference, tt.bookingID, tt.checkIn)
			if token == "" {
				t.Fatalf("expected non-empty token")
			}

			verifyToken := token
			if tt.tamperTok {
				verifyToken = "tampered" + token[8:]
			}

			valid := VerifyVoucherToken(tt.secret, tt.reference, tt.bookingID, tt.checkIn, verifyToken)
			if valid != tt.wantValid {
				t.Errorf("VerifyVoucherToken() = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestGenerateQRPNG(t *testing.T) {
	tests := []struct {
		name    string
		content string
		size    int
		wantErr bool
	}{
		{
			name:    "Standard QR generation",
			content: "https://pulangkeuttara.id/voucher/verify?ref=PKU-1234",
			size:    200,
			wantErr: false,
		},
		{
			name:    "Default size fallback",
			content: "test-content",
			size:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			png, err := GenerateQRPNG(tt.content, tt.size)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateQRPNG() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				// Check PNG header signature \x89PNG\r\n\x1a\n
				pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
				if len(png) < 8 || !bytes.Equal(png[:8], pngHeader) {
					t.Errorf("GenerateQRPNG() produced invalid PNG header")
				}
			}
		})
	}
}

func TestCalculatePBJTTaxes(t *testing.T) {
	tests := []struct {
		name         string
		totalPaidIDR int64
		wantZero     bool
	}{
		{
			name:         "Zero or negative total",
			totalPaidIDR: 0,
			wantZero:     true,
		},
		{
			name:         "Standard room rate 1,250,000 IDR",
			totalPaidIDR: 1250000,
			wantZero:     false,
		},
		{
			name:         "Even divider rate 2,420,000 IDR (DPP 2M)",
			totalPaidIDR: 2420000,
			wantZero:     false,
		},
		{
			name:         "High rate suite 5,750,000 IDR",
			totalPaidIDR: 5750000,
			wantZero:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dpp, service, pbjt := CalculatePBJTTaxes(tt.totalPaidIDR)
			if tt.wantZero {
				if dpp != 0 || service != 0 || pbjt != 0 {
					t.Errorf("expected all zeros, got dpp=%d, service=%d, pbjt=%d", dpp, service, pbjt)
				}
				return
			}

			// Invariant: sum of components MUST exactly equal totalPaidIDR
			if dpp+service+pbjt != tt.totalPaidIDR {
				t.Errorf("invariance violated: dpp(%d) + service(%d) + pbjt(%d) = %d != total(%d)",
					dpp, service, pbjt, dpp+service+pbjt, tt.totalPaidIDR)
			}

			if dpp <= 0 || service <= 0 || pbjt <= 0 {
				t.Errorf("expected positive components, got dpp=%d, service=%d, pbjt=%d", dpp, service, pbjt)
			}
		})
	}
}

func TestFormatRupiah(t *testing.T) {
	tests := []struct {
		name string
		val  int64
		want string
	}{
		{"Zero", 0, "0"},
		{"Hundreds", 500, "500"},
		{"Thousands", 1000, "1.000"},
		{"Ten thousands", 25000, "25.000"},
		{"Millions", 1250000, "1.250.000"},
		{"Negative millions", -2500000, "-2.500.000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRupiah(tt.val)
			if got != tt.want {
				t.Errorf("formatRupiah(%d) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestGenerateVoucherPDF(t *testing.T) {
	qr, err := GenerateQRPNG("test-qr-content", 150)
	if err != nil {
		t.Fatalf("failed to generate test qr: %v", err)
	}

	tests := []struct {
		name    string
		data    VoucherData
		qrPNG   []byte
		wantErr bool
	}{
		{
			name: "Complete voucher with QR",
			data: VoucherData{
				Reference:     "PKU-20261004-0001",
				BookingID:     "bkg-12345",
				GuestName:     "Budi Santoso",
				GuestEmail:    "budi.santoso@example.com",
				GuestPhone:    "+6281234567890",
				RoomTypeName:  "Deluxe King",
				RatePlanName:  "Breakfast Included",
				Inclusions:    "Buffet Breakfast for 2, Afternoon Tea",
				CheckIn:       time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC),
				CheckOut:      time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC),
				Nights:        2,
				NumRooms:      1,
				Adults:        2,
				Children:      1,
				TotalPaidIDR:  2500000,
				PaymentMethod: "BCA Virtual Account",
				PaidAt:        time.Now(),
				QRToken:       "a1b2c3d4e5f67890",
			},
			qrPNG:   qr,
			wantErr: false,
		},
		{
			name: "Voucher without QR and empty inclusions fallback",
			data: VoucherData{
				Reference:     "PKU-20261004-0002",
				BookingID:     "bkg-67890",
				GuestName:     "Siti Rahma",
				GuestEmail:    "siti@example.com",
				GuestPhone:    "+628987654321",
				RoomTypeName:  "Superior Twin",
				RatePlanName:  "Room Only",
				Inclusions:    "",
				CheckIn:       time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC),
				CheckOut:      time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC),
				Nights:        1,
				NumRooms:      1,
				Adults:        1,
				Children:      0,
				TotalPaidIDR:  1100000,
				PaymentMethod: "QRIS",
				PaidAt:        time.Now(),
				QRToken:       "f9e8d7c6b5a43210",
			},
			qrPNG:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdfBytes, err := GenerateVoucherPDF(tt.data, tt.qrPNG)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateVoucherPDF() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(pdfBytes) == 0 {
					t.Fatalf("expected non-empty PDF bytes")
				}
				// Verify PDF magic header %PDF-
				if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
					t.Errorf("GenerateVoucherPDF() output did not start with %%PDF-")
				}
			}
		})
	}
}

func TestGenerateInvoicePDF(t *testing.T) {
	dpp, svc, pbjt := CalculatePBJTTaxes(2420000)

	tests := []struct {
		name    string
		data    InvoiceData
		wantErr bool
	}{
		{
			name: "Valid official PBJT invoice",
			data: InvoiceData{
				InvoiceNumber:     "INV-20261004-0001",
				Reference:         "PKU-20261004-0001",
				BookingID:         "bkg-12345",
				GuestName:         "Budi Santoso",
				GuestEmail:        "budi.santoso@example.com",
				RoomTypeName:      "Deluxe King",
				CheckIn:           time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC),
				CheckOut:          time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC),
				Nights:            2,
				NumRooms:          1,
				NetRoomChargesIDR: dpp,
				ServiceChargeIDR:  svc,
				PBJTTaxIDR:        pbjt,
				TotalPaidIDR:      2420000,
				PaymentMethod:     "BCA Virtual Account",
				PaidAt:            time.Now(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdfBytes, err := GenerateInvoicePDF(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateInvoicePDF() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(pdfBytes) == 0 {
					t.Fatalf("expected non-empty PDF bytes")
				}
				if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
					t.Errorf("GenerateInvoicePDF() output did not start with %%PDF-")
				}
			}
		})
	}
}
