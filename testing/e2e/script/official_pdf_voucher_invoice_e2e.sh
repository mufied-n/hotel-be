#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Candidate B - Official PDF Voucher & Sleman PBJT Tax Invoice Engine
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Skenario Pengujian:
# 1. Verification of Go In-Process E2E Suite (E2E-75 & E2E-76)
# 2. Ephemeral Live Server Lifecycle Boot
# 3. Unpaid Booking Document Access Guard (400 RECEIPT_NOT_AVAILABLE)
# 4. Confirmation Payment via Mock Gateway (/fake-pay/:ref)
# 5. Confirmation Voucher PDF Download (Front Desk Staff Access)
# 6. Sleman PBJT Tax Invoice PDF Download (Accounting/Finance Access)
# 7. RBAC Least-Privilege Security Guard: Receptionist Forbidden on Tax Invoice (403)
# 8. Front Desk Express Check-in: Valid HMAC QR Signature Verification (200 OK)
# 9. Front Desk Express Check-in: Tampered Signature Rejection (400 INVALID_QR_SIGNATURE)
# 10. Front Desk Express Check-in: Missing Parameters Validation (400 MISSING_PARAMETERS)
# 11. Front Desk Express Check-in: Non-Existent Booking Lookup (404 BOOKING_NOT_FOUND)
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

TOTAL=0
PASSED=0
FAILED=0

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

assert_eq() {
    local desc="$1"
    local expected="$2"
    local actual="$3"
    TOTAL=$((TOTAL + 1))
    if [ "$actual" == "$expected" ]; then
        echo -e "  ${GREEN}✓${NC} ${desc} (Expected: ${expected})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}✗${NC} ${desc} — Expected: '${expected}', Got: '${actual}'"
        FAILED=$((FAILED + 1))
    fi
}

assert_contains() {
    local desc="$1"
    local needle="$2"
    local haystack="$3"
    TOTAL=$((TOTAL + 1))
    if echo "$haystack" | grep -qi "$needle"; then
        echo -e "  ${GREEN}✓${NC} ${desc} (Contains: ${needle})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}✗${NC} ${desc} — Needle '${needle}' not found in:\n${haystack}"
        FAILED=$((FAILED + 1))
    fi
}

echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  E2E TEST: OFFICIAL PDF VOUCHER & SLEMAN PBJT TAX INVOICE ENGINE (CANDIDATE B)${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

# ------------------------------------------------------------------------------
# 1. Run Automated In-Process Go E2E Suite (E2E-75 & E2E-76)
# ------------------------------------------------------------------------------
echo -e "${CYAN}1. Menjalankan Go In-Process E2E Test Suite (E2E-75 & E2E-76)...${NC}"
if (cd "${REPO_ROOT}" && go test -v ./testing/e2e/script -run 'TestEndToEndHotelBookingRBACLifecycle/(E2E-75|E2E-76)'); then
    echo -e "  ${GREEN}✓${NC} Go E2E Suite (E2E-75 & E2E-76) passed successfully"
    TOTAL=$((TOTAL + 2))
    PASSED=$((PASSED + 2))
else
    echo -e "  ${RED}✗${NC} Go E2E Suite failed"
    TOTAL=$((TOTAL + 2))
    FAILED=$((FAILED + 2))
fi

# ------------------------------------------------------------------------------
# 2. Boot Ephemeral Server or Use Provided BASE_URL
# ------------------------------------------------------------------------------
TMP_DIR="$(mktemp -d /tmp/e2e_docgen_XXXXXX)"
EPHEMERAL_PID=""

cleanup() {
    if [ -n "$EPHEMERAL_PID" ]; then
        kill -TERM "$EPHEMERAL_PID" 2>/dev/null || true
    fi
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

BASE_URL="${API_BASE_URL:-}"
if [ -z "${BASE_URL}" ]; then
    EPHEMERAL_PORT=28092
    BASE_URL="http://127.0.0.1:${EPHEMERAL_PORT}"
    echo -e "\n${CYAN}2. Memulai Ephemeral Live Server pada port ${EPHEMERAL_PORT}...${NC}"
    (cd "${REPO_ROOT}" && RUN_EPHEMERAL_E2E_SERVER_PORT="${EPHEMERAL_PORT}" go test -v ./testing/e2e/script -run TestEphemeralServerRunner > "$TMP_DIR/ephemeral.log" 2>&1) &
    EPHEMERAL_PID=$!

    # Wait for server ready
    READY=0
    for _ in $(seq 1 40); do
        if curl -s -f "${BASE_URL}/healthz" > /dev/null 2>&1; then
            READY=1
            break
        fi
        sleep 0.2
    done

    if [ "$READY" -ne 1 ]; then
        echo -e "${RED}Ephemeral server gagal aktif! Log:${NC}"
        cat "$TMP_DIR/ephemeral.log" || true
        exit 1
    fi
    echo -e "  ${GREEN}✓${NC} Ephemeral live server berhasil aktif di ${BASE_URL}"
fi

# ------------------------------------------------------------------------------
# 3. Guard: Unpaid Booking Rejection (RECEIPT_NOT_AVAILABLE)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}3. Penjagaan Reservasi Belum Lunas (RECEIPT_NOT_AVAILABLE)...${NC}"
UNPAID_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/bk-e2e-001/voucher.pdf" \
    -H "Authorization: Bearer receptionist")
UNPAID_STATUS=$(echo "$UNPAID_RESP" | tail -n1)
UNPAID_BODY=$(echo "$UNPAID_RESP" | head -n -1)
assert_eq "Unduh voucher pada reservasi pending ditolak 400 Bad Request" "400" "$UNPAID_STATUS"
assert_contains "Error memuat kode RECEIPT_NOT_AVAILABLE" "RECEIPT_NOT_AVAILABLE" "$UNPAID_BODY"

# ------------------------------------------------------------------------------
# 4. Confirm Booking via Payment Gateway (/fake-pay/:ref)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}4. Pelunasan Pembayaran Reservasi via Mock Gateway (/fake-pay)...${NC}"
PAY_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/fake-pay/bk-e2e-001")
PAY_STATUS=$(echo "$PAY_RESP" | tail -n1)
assert_eq "Pelunasan fake-pay returns 200 OK" "200" "$PAY_STATUS"

# ------------------------------------------------------------------------------
# 5. Download Confirmation Voucher PDF as Front Desk Staff
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}5. Unduh Confirmation Voucher PDF oleh Front Desk (FR-01)...${NC}"
VOUCHER_FILE="$TMP_DIR/voucher.pdf"
VOUCHER_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/bk-e2e-001/voucher.pdf" \
    -H "Authorization: Bearer receptionist" \
    -o "$VOUCHER_FILE")
VOUCHER_STATUS=$(echo "$VOUCHER_RESP" | tail -n1)
assert_eq "Staff Receptionist unduh voucher PDF returns 200 OK" "200" "$VOUCHER_STATUS"

if [ -f "$VOUCHER_FILE" ] && head -c 5 "$VOUCHER_FILE" | grep -q "%PDF-"; then
    echo -e "  ${GREEN}✓${NC} Berkas Confirmation Voucher adalah PDF valid (%PDF- header)"
    TOTAL=$((TOTAL + 1))
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}✗${NC} Berkas Confirmation Voucher bukan PDF valid"
    TOTAL=$((TOTAL + 1))
    FAILED=$((FAILED + 1))
fi

# ------------------------------------------------------------------------------
# 6. Download Sleman PBJT Tax Invoice PDF as Finance Staff
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}6. Unduh Faktur Pajak Daerah PBJT Sleman oleh Finance (FR-02)...${NC}"
INVOICE_FILE="$TMP_DIR/invoice.pdf"
INVOICE_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/bk-e2e-001/invoice.pdf" \
    -H "Authorization: Bearer finance" \
    -o "$INVOICE_FILE")
INVOICE_STATUS=$(echo "$INVOICE_RESP" | tail -n1)
assert_eq "Staff Finance unduh faktur pajak PBJT PDF returns 200 OK" "200" "$INVOICE_STATUS"

if [ -f "$INVOICE_FILE" ] && head -c 5 "$INVOICE_FILE" | grep -q "%PDF-"; then
    echo -e "  ${GREEN}✓${NC} Berkas Faktur Pajak Daerah PBJT adalah PDF valid (%PDF- header)"
    TOTAL=$((TOTAL + 1))
    PASSED=$((PASSED + 1))
else
    echo -e "  ${RED}✗${NC} Berkas Faktur Pajak Daerah PBJT bukan PDF valid"
    TOTAL=$((TOTAL + 1))
    FAILED=$((FAILED + 1))
fi

# ------------------------------------------------------------------------------
# 7. RBAC Security Guard: Receptionist Forbidden on Tax Invoice
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}7. Penegakan Kebijakan RBAC: Resepsionis Dilarang Unduh Faktur Pajak (FR-02)...${NC}"
FORBIDDEN_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/bookings/bk-e2e-001/invoice.pdf" \
    -H "Authorization: Bearer receptionist")
FORBIDDEN_STATUS=$(echo "$FORBIDDEN_RESP" | tail -n1)
assert_eq "Resepsionis ditolak 403 Forbidden saat mengakses faktur pajak" "403" "$FORBIDDEN_STATUS"

# ------------------------------------------------------------------------------
# 8. Front Desk Express Check-in: Verify Voucher with Valid HMAC Signature
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}8. Validasi QR Code Voucher Meja Depan / Express Check-in (FR-03)...${NC}"
CHECK_IN_DATE=$(date +%Y-%m-%d)
VALID_TOKEN=$(echo -n "PKU-20261003-BKE2E001:bk-e2e-001:${CHECK_IN_DATE}" | openssl dgst -sha256 -hmac "pku-voucher-qr-hmac-secret-key-2026" | awk '{print $NF}')

VERIFY_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/verify-voucher?ref=PKU-20261003-BKE2E001&token=${VALID_TOKEN}&id=bk-e2e-001" \
    -H "Authorization: Bearer receptionist")
VERIFY_STATUS=$(echo "$VERIFY_RESP" | tail -n1)
VERIFY_BODY=$(echo "$VERIFY_RESP" | head -n -1)
assert_eq "Verifikasi voucher valid returns 200 OK" "200" "$VERIFY_STATUS"
assert_contains "Payload verifikasi memuat SIGNATURE_VERIFIED" "SIGNATURE_VERIFIED" "$VERIFY_BODY"
assert_contains "Payload verifikasi memuat nama tamu" "Budi Santoso" "$VERIFY_BODY"

# ------------------------------------------------------------------------------
# 9. Front Desk Express Check-in: Tampered HMAC Signature Rejection
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}9. Penolakan Tanda Tangan QR Voucher yang Telah Dimodifikasi / Tampered (FR-03)...${NC}"
TAMPER_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/verify-voucher?ref=PKU-20261003-BKE2E001&token=palsu_tampered_signature_123&id=bk-e2e-001" \
    -H "Authorization: Bearer receptionist")
TAMPER_STATUS=$(echo "$TAMPER_RESP" | tail -n1)
TAMPER_BODY=$(echo "$TAMPER_RESP" | head -n -1)
assert_eq "Verifikasi QR tampered ditolak 400 Bad Request" "400" "$TAMPER_STATUS"
assert_contains "Error memuat kode INVALID_QR_SIGNATURE" "INVALID_QR_SIGNATURE" "$TAMPER_BODY"

# ------------------------------------------------------------------------------
# 10. Front Desk Express Check-in: Missing Parameters Validation
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}10. Validasi Parameter Wajib Verifikasi Voucher (FR-03)...${NC}"
MISSING_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/verify-voucher" \
    -H "Authorization: Bearer receptionist")
MISSING_STATUS=$(echo "$MISSING_RESP" | tail -n1)
MISSING_BODY=$(echo "$MISSING_RESP" | head -n -1)
assert_eq "Permintaan tanpa parameter ditolak 400 Bad Request" "400" "$MISSING_STATUS"
assert_contains "Error memuat kode MISSING_PARAMETERS" "MISSING_PARAMETERS" "$MISSING_BODY"

# ------------------------------------------------------------------------------
# 11. Front Desk Express Check-in: Non-Existent Booking Reference
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}11. Penanganan Voucher Reservasi Tidak Ditemukan (FR-03)...${NC}"
NOTFOUND_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/verify-voucher?ref=PKU-99999999-NOTFOUND&token=${VALID_TOKEN}" \
    -H "Authorization: Bearer receptionist")
NOTFOUND_STATUS=$(echo "$NOTFOUND_RESP" | tail -n1)
NOTFOUND_BODY=$(echo "$NOTFOUND_RESP" | head -n -1)
assert_eq "Voucher tidak ditemukan ditolak 404 Not Found" "404" "$NOTFOUND_STATUS"
assert_contains "Error memuat kode BOOKING_NOT_FOUND" "BOOKING_NOT_FOUND" "$NOTFOUND_BODY"

# ------------------------------------------------------------------------------
# Summary & Exit
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  HASIL PENGUJIAN E2E: OFFICIAL PDF VOUCHER & PBJT TAX INVOICE ENGINE          ${NC}"
echo -e "${BLUE}==============================================================================${NC}"
echo -e "Total Pengujian  : ${TOTAL}"
echo -e "Berhasil (Pass)  : ${GREEN}${PASSED}${NC}"
echo -e "Gagal (Fail)     : ${RED}${FAILED}${NC}"
echo -e "Tingkat Kelulusan: $(( PASSED * 100 / TOTAL ))%\n"

if [ "$FAILED" -eq 0 ]; then
    echo -e "${GREEN}SELURUH PENGUJIAN E2E KANDIDAT B LULUS 100%!${NC}\n"
    exit 0
else
    echo -e "${RED}BEBERAPA PENGUJIAN E2E GAGAL! SILAKAN PERIKSA LOG DI ATAS.${NC}\n"
    exit 1
fi
