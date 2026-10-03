package api

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())

	// Register tag name func to read JSON tag for clean field naming
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		if name != "" {
			return name
		}
		return fld.Name
	})
}

// validateDTO memvalidasi struct request menggunakan validator/v10.
// Jika terjadi pelanggaran, respon RFC 7807 dituliskan otomatis dan mengembalikan false.
func validateDTO(c *gin.Context, s any) bool {
	if err := validate.StructCtx(c.Request.Context(), s); err != nil {
		handleValidationError(c, err)
		return false
	}
	return true
}

// handleValidationError memetakan error validasi go-playground/validator/v10 ke format RFC 7807 Problem Details.
// Menjamin kompatibilitas mundur 100% dengan kode error spesifik yang diharapkan client dan test suite.
func handleValidationError(c *gin.Context, err error) {
	var valErrors validator.ValidationErrors
	if !errors.As(err, &valErrors) || len(valErrors) == 0 {
		httpErrorCode(c, http.StatusBadRequest, "format payload json tidak valid", "INVALID_PAYLOAD")
		return
	}

	first := valErrors[0]
	field := first.Field()
	tag := first.Tag()

	// Default problem details
	code := "INVALID_PAYLOAD"
	detail := fmt.Sprintf("field '%s' gagal validasi aturan '%s'", field, tag)

	switch field {
	case "rooms":
		code = "INVALID_ROOM_COUNT"
		detail = "rooms must be between 1 and 8"
	case "adults":
		code = "INVALID_GUEST_COUNT"
		detail = "adults must be at least 1"
	case "children":
		code = "INVALID_GUEST_COUNT"
		detail = "children cannot be negative"
	case "child_ages":
		code = "INVALID_CHILD_AGE"
		detail = "child age must be between 0 and 17"
	case "room_type_id", "check_in", "check_out":
		code = "INVALID_DATE_FORMAT"
		detail = "check_in and check_out (YYYY-MM-DD) are required"
	case "code", "name", "max_capacity", "base_price_minor":
		code = "INVALID_ROOM_DATA"
		detail = "kode, nama, kapasitas, dan harga dasar wajib diisi"
	case "amount_minor":
		code = "INVALID_AMOUNT"
		detail = "nominal refund harus lebih dari 0"
	case "reason":
		code = "REASON_REQUIRED"
		detail = "alasan refund wajib diisi minimal 5 karakter"
	case "action":
		code = "ACTION_REQUIRED"
		detail = "tindakan resolusi (action) wajib diisi"
	case "target_room_number":
		code = "INVALID_ROOM_NUMBER"
		detail = "nomor kamar tujuan wajib disertakan"
	case "reason_category":
		code = "INVALID_REASON_CATEGORY"
		detail = "kategori alasan pemindahan kamar tidak valid"
	case "additional_nights":
		code = "INVALID_ADDITIONAL_NIGHTS"
		detail = "additional_nights harus lebih dari 0"
	case "shift":
		code = "INVALID_SHIFT"
		detail = "shift wajib diisi"
	case "to_status":
		code = "INVALID_STATUS"
		detail = "to_status wajib diisi"
	}

	writeProblemDetails(c, http.StatusBadRequest, "Bad Request", detail, code)
}
