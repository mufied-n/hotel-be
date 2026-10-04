#!/usr/bin/env bash
# ==============================================================================
# Script: feature_flags_e2e.sh
# Deskripsi: Pengujian E2E Otomatis untuk Hospitality & Multi-Channel Feature Flags (FR-FF-08)
#            Memverifikasi dynamic runtime kill-switch dan live toggling via admin API.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

COLOR_GREEN='\033[0;32m'
COLOR_RED='\033[0;31m'
COLOR_BLUE='\033[0;34m'
COLOR_RESET='\033[0m'

log_info() {
    echo -e "${COLOR_BLUE}[INFO]${COLOR_RESET} $1"
}

log_pass() {
    echo -e "  ${COLOR_GREEN}✓${COLOR_RESET} $1"
    PASSED_TESTS=$((PASSED_TESTS + 1))
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
}

log_fail() {
    echo -e "  ${COLOR_RED}✗${COLOR_RESET} $1"
    FAILED_TESTS=$((FAILED_TESTS + 1))
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
}

assert_status() {
    local desc="$1"
    local expected="$2"
    local actual="$3"
    if [ "$actual" -eq "$expected" ]; then
        log_pass "$desc (Expected: $expected)"
    else
        log_fail "$desc (Expected: $expected, Got: $actual)"
    fi
}

assert_contains() {
    local desc="$1"
    local needle="$2"
    local haystack="$3"
    if echo "$haystack" | grep -q "$needle"; then
        log_pass "$desc (Contains: '$needle')"
    else
        log_fail "$desc (Missing: '$needle' in '$haystack')"
    fi
}

echo "=============================================================================="
echo "  E2E TEST AUTOMATION: HOSPITALITY & CHANNELS FEATURE FLAGS (FR-FF-08)        "
echo "=============================================================================="
echo "Target: Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)"
echo "Waktu : $(date)"
echo ""

# ------------------------------------------------------------------------------
# 1. Jalankan Go In-Process E2E Sub-test (E2E-82)
# ------------------------------------------------------------------------------
log_info "1. Menjalankan Go In-Process E2E Suite (E2E-82)..."
if go test -v -run "TestEndToEndHotelBookingRBACLifecycle/E2E-82" ./testing/e2e/script/... > /tmp/go_ff_e2e.log 2>&1; then
    log_pass "Go In-Process E2E-82 (Feature Flags Kill-Switch & Admin Toggling) lulus 100%"
else
    cat /tmp/go_ff_e2e.log
    log_fail "Go In-Process E2E-82 gagal"
fi

# ------------------------------------------------------------------------------
# 2. Setup Live Ephemeral Server
# ------------------------------------------------------------------------------
TMP_DIR=$(mktemp -d)
EPHEMERAL_PID=""

cleanup() {
    log_info "Membersihkan proses live server..."
    if [ -n "$EPHEMERAL_PID" ]; then
        kill -9 "$EPHEMERAL_PID" 2>/dev/null || true
        wait "$EPHEMERAL_PID" 2>/dev/null || true
    fi
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

PORT=28095
BASE_URL="http://127.0.0.1:${PORT}"

log_info "2. Memulai Ephemeral Live Server pada port ${PORT}..."
(cd "${REPO_ROOT}" && RUN_EPHEMERAL_E2E_SERVER_PORT="${PORT}" go test -v ./testing/e2e/script -run TestEphemeralServerRunner > "$TMP_DIR/ephemeral.log" 2>&1) &
EPHEMERAL_PID=$!

READY=0
for _ in $(seq 1 40); do
    if curl -s -f "${BASE_URL}/healthz" > /dev/null 2>&1; then
        READY=1
        break
    fi
    sleep 0.2
done

if [ "$READY" -ne 1 ]; then
    echo "Server gagal menyala. Log:"
    cat "$TMP_DIR/ephemeral.log" || true
    exit 1
fi
log_pass "Ephemeral live server berhasil aktif di ${BASE_URL}"

# ------------------------------------------------------------------------------
# 3. GM Admin Inspect Daftar Feature Flags Baru
# ------------------------------------------------------------------------------
log_info "3. Pengujian GM Admin Menginspeksi Daftar Feature Flags..."
LIST_RESP=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/admin/feature-flags" \
    -H "Authorization: Bearer gm_admin")
HTTP_CODE=$(echo "$LIST_RESP" | tail -n1)
BODY=$(echo "$LIST_RESP" | sed '$d')

assert_status "GM Admin membaca daftar feature flags" 200 "$HTTP_CODE"
assert_contains "Memuat flag ff_channel_sync_integration" "ff_channel_sync_integration" "$BODY"
assert_contains "Memuat flag ff_realtime_event_hub" "ff_realtime_event_hub" "$BODY"
assert_contains "Memuat flag ff_last_room_safeguards" "ff_last_room_safeguards" "$BODY"
assert_contains "Memuat flag ff_dynamic_rates_calendar" "ff_dynamic_rates_calendar" "$BODY"
assert_contains "Memuat flag ff_official_pdf_voucher" "ff_official_pdf_voucher" "$BODY"

# ------------------------------------------------------------------------------
# 4. Emergency Kill-Switch: Matikan ff_channel_sync_integration
# ------------------------------------------------------------------------------
log_info "4. Pengujian Emergency Kill-Switch: Matikan ff_channel_sync_integration..."
DISABLE_RESP=$(curl -s -w "\n%{http_code}" -X PUT "${BASE_URL}/api/v1/admin/feature-flags/ff_channel_sync_integration" \
    -H "Authorization: Bearer gm_admin" \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}')
HTTP_CODE=$(echo "$DISABLE_RESP" | tail -n1)
BODY=$(echo "$DISABLE_RESP" | sed '$d')

assert_status "GM Admin menonaktifkan ff_channel_sync_integration" 200 "$HTTP_CODE"
assert_contains "Status respons updated" "updated" "$BODY"

# Coba akses webhook yang telah dimatikan
WEBHOOK_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/channel-events" \
    -H "Content-Type: application/json" \
    -d '{}')
HTTP_CODE=$(echo "$WEBHOOK_RESP" | tail -n1)
BODY=$(echo "$WEBHOOK_RESP" | sed '$d')

assert_status "Akses ke webhook tertutup dengan 503 Service Unavailable" 503 "$HTTP_CODE"
assert_contains "Respons memuat kode FEATURE_DISABLED" "FEATURE_DISABLED" "$BODY"

# ------------------------------------------------------------------------------
# 5. Pemulihan Flag: Nyalakan Kembali ff_channel_sync_integration
# ------------------------------------------------------------------------------
log_info "5. Pemulihan Flag: Mengaktifkan Kembali ff_channel_sync_integration..."
ENABLE_RESP=$(curl -s -w "\n%{http_code}" -X PUT "${BASE_URL}/api/v1/admin/feature-flags/ff_channel_sync_integration" \
    -H "Authorization: Bearer gm_admin" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}')
HTTP_CODE=$(echo "$ENABLE_RESP" | tail -n1)
assert_status "GM Admin mengaktifkan kembali ff_channel_sync_integration" 200 "$HTTP_CODE"

# Coba akses webhook kembali (sekarang harus diproses oleh handler, ditolak 400 Bad Request karena payload kosong, BUKAN 503)
WEBHOOK_RESTORE_RESP=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/channel-events" \
    -H "Content-Type: application/json" \
    -d '{}')
HTTP_CODE=$(echo "$WEBHOOK_RESTORE_RESP" | tail -n1)
assert_status "Webhook diproses normal oleh handler (400 Bad Request, bukan 503)" 400 "$HTTP_CODE"

# ------------------------------------------------------------------------------
# 6. Ringkasan Hasil
# ------------------------------------------------------------------------------
echo ""
echo "=============================================================================="
echo "  RINGKASAN HASIL PENGUJIAN E2E: FEATURE FLAGS SYSTEM                         "
echo "=============================================================================="
echo "Total Asersi: $TOTAL_TESTS"
echo "Lulus (Passed) : $PASSED_TESTS"
echo "Gagal (Failed) : $FAILED_TESTS"
echo ""

if [ "$FAILED_TESTS" -eq 0 ]; then
    echo -e "${COLOR_GREEN}★★★ SELURUH PENGUJIAN FEATURE FLAGS BERHASIL (100% PASS) ★★★${COLOR_RESET}"
    exit 0
else
    echo -e "${COLOR_RED}✗ BEBERAPA PENGUJIAN GAGAL (FAILED)${COLOR_RESET}"
    exit 1
fi
