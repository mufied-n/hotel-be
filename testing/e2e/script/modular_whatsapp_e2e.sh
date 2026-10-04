#!/usr/bin/env bash
# ==============================================================================
# Script: modular_whatsapp_e2e.sh
# Deskripsi: Pengujian E2E Otomatis untuk Modular WhatsApp Notifier Engine (F11)
#            Memverifikasi pluggability provider: Log, Generic HTTP, Twilio, Meta WABA.
# ==============================================================================

set -euo pipefail

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

echo "=============================================================================="
echo "  E2E TEST AUTOMATION: MODULAR WHATSAPP NOTIFIER ENGINE (F11)                 "
echo "=============================================================================="
echo "Target: Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)"
echo "Waktu : $(date)"
echo ""

# ------------------------------------------------------------------------------
# 1. Jalankan Go In-Process E2E Sub-test (E2E-81)
# ------------------------------------------------------------------------------
log_info "1. Menjalankan Go In-Process E2E Suite (E2E-81: Modular Multi-Provider WhatsApp)..."
if go test -v -run "TestEndToEndHotelBookingRBACLifecycle/E2E-81" ./testing/e2e/script/... > /tmp/go_wa_e2e.log 2>&1; then
    log_pass "Go In-Process E2E-81 (Log, Generic HTTP, Twilio, Meta WABA) lulus 100%"
else
    cat /tmp/go_wa_e2e.log
    log_fail "Go In-Process E2E-81 gagal"
fi

# ------------------------------------------------------------------------------
# 2. Verifikasi Standar Test Coverage Notifier Package (≥80%)
# ------------------------------------------------------------------------------
log_info "2. Menjalankan Unit Tests & Code Coverage Notifier..."
COVERAGE_OUTPUT=$(go test -cover ./internal/adapter/notifier/...)
echo "$COVERAGE_OUTPUT"

COVERAGE_NUM=$(echo "$COVERAGE_OUTPUT" | grep -o '[0-9.]*%' | head -n1 | tr -d '%')
if (( $(echo "$COVERAGE_NUM >= 80.0" | bc -l) )); then
    log_pass "Coverage internal/adapter/notifier mencapai ${COVERAGE_NUM}% (Target ≥ 80%)"
else
    log_fail "Coverage internal/adapter/notifier (${COVERAGE_NUM}%) di bawah standar 80%"
fi

# ------------------------------------------------------------------------------
# 3. Static Analysis & Linter Verification
# ------------------------------------------------------------------------------
log_info "3. Menjalankan Static Analysis (go vet)..."
if go vet ./internal/adapter/notifier/... ./cmd/server/... ./testing/e2e/script/...; then
    log_pass "Static analysis go vet bersih (0 issues)"
else
    log_fail "go vet mendeteksi masalah"
fi

# ------------------------------------------------------------------------------
# 4. Data Race Detection (-race)
# ------------------------------------------------------------------------------
log_info "4. Menjalankan Data Race Detector (-race)..."
if go test -race ./internal/adapter/notifier/... > /dev/null 2>&1; then
    log_pass "Race detector bersih (0 data races)"
else
    log_fail "Race detector mendeteksi data race"
fi

# ------------------------------------------------------------------------------
# 5. Ringkasan Hasil
# ------------------------------------------------------------------------------
echo ""
echo "=============================================================================="
echo "  RINGKASAN HASIL PENGUJIAN E2E: MODULAR WHATSAPP ENGINE                      "
echo "=============================================================================="
echo "Total Asersi: $TOTAL_TESTS"
echo "Lulus (Passed) : $PASSED_TESTS"
echo "Gagal (Failed) : $FAILED_TESTS"
echo ""

if [ "$FAILED_TESTS" -eq 0 ]; then
    echo -e "${COLOR_GREEN}★★★ SELURUH PENGUJIAN MODULAR WHATSAPP BERHASIL (100% PASS) ★★★${COLOR_RESET}"
    exit 0
else
    echo -e "${COLOR_RED}✗ BEBERAPA PENGUJIAN GAGAL (FAILED)${COLOR_RESET}"
    exit 1
fi
