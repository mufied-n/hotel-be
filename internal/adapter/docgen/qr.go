package docgen

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/skip2/go-qrcode"
)

const (
	// DefaultQRSecret adalah kunci HMAC default untuk validasi voucher hotel.
	DefaultQRSecret = "pku-voucher-qr-hmac-secret-key-2026"
)

var (
	ErrInvalidQRToken = errors.New("docgen: invalid or tampered qr token")
)

// SignVoucherToken menghasilkan token tanda tangan digital HMAC-SHA256 untuk voucher.
func SignVoucherToken(secret, reference, bookingID, checkIn string) string {
	if secret == "" {
		secret = DefaultQRSecret
	}
	mac := hmac.New(sha256.New, []byte(secret))
	payload := fmt.Sprintf("%s:%s:%s", reference, bookingID, checkIn)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyVoucherToken memvalidasi keaslian token tanda tangan digital menggunakan perbandingan konstan time (anti-timing attack).
func VerifyVoucherToken(secret, reference, bookingID, checkIn, token string) bool {
	expected := SignVoucherToken(secret, reference, bookingID, checkIn)
	return hmac.Equal([]byte(expected), []byte(token))
}

// GenerateQRPNG menghasilkan data gambar QR code dalam format PNG byte stream.
func GenerateQRPNG(content string, size int) ([]byte, error) {
	if size <= 0 {
		size = 200
	}
	png, err := qrcode.Encode(content, qrcode.Medium, size)
	if err != nil {
		return nil, fmt.Errorf("docgen: generate qr code png: %w", err)
	}
	return png, nil
}
