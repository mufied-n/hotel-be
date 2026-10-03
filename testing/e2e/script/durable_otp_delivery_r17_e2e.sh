#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Durable OTP Delivery & Transactional Outbox (BE-R17)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Standar Pengujian:
# 1. Atomisitas Transaksional Outbox:
#    - Request OTP menyisipkan rekaman di guest_auth_challenges DAN outbox ('guest.otp_dispatch')
#      dalam transaksi database yang sama.
# 2. Pemrosesan Asinkron Outbox Relay:
#    - Worker OutboxRelay membaca event 'guest.otp_dispatch' (FOR UPDATE SKIP LOCKED),
#      mengirim via notifier, dan memperbarui status outbox menjadi 'done'.
# 3. Idempotency Key Berbasis Challenge ID:
#    - Retry event memakai Idempotency-Key: otp-challenge-<id> yang sama.
#    - Tantangan baru memakai key yang berbeda (mencegah kolisi satu menit).
# 4. Proteksi Kode Kedaluwarsa (Anti-Stale Delivery):
#    - Event outbox dengan waktu expires_at lampau didiskualifikasi tanpa kirim email.
# 5. Proteksi Kode Terverifikasi (Already Verified Discard):
#    - Jika tantangan sudah diverifikasi sebelumnya, pengiriman dibatalkan.
# 6. Kejujuran API & Anti-Enumerasi:
#    - HTTP 200 OK mengembalikan delivery_status: accepted dan pesan netral.
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:28080}"
PG_CONN="${PG_CONN:-postgres://postgres:postgres@localhost:25432/hotel_test?sslmode=disable}"

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
        echo -e "  ${RED}✗${NC} ${desc} — Did not find: '${needle}' in payload"
        echo -e "       Content: ${haystack}"
        FAILED=$((FAILED + 1))
    fi
}

echo -e "\n${BLUE}=================================================================${NC}"
echo -e "${BLUE}  E2E Test: BE-R17 Durable OTP Delivery & Transactional Outbox   ${NC}"
echo -e "${BLUE}=================================================================${NC}"

# 0. Healthcheck
echo -e "\n${YELLOW}--- 0. Healthcheck Service ---${NC}"
HEALTH=$(curl -s "${BASE_URL}/healthz")
assert_contains "Service Healthcheck OK" '"status":"ok"' "$HEALTH"

TEST_EMAIL="durable.otp.$(date +%s%N)@example.id"

# ------------------------------------------------------------------------------
# Skenario 1: Request Challenge, Respon Jujur & Atomisitas Transaksional Outbox
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 1. OTP Challenge Request & Transactional Outbox Atomicity ---${NC}"

CHAL_RES=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/challenge" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${TEST_EMAIL}\"}")
CHAL_STATUS=$(echo "$CHAL_RES" | tail -n1)
CHAL_BODY=$(echo "$CHAL_RES" | head -n-1)

assert_eq "Request challenge mengembalikan HTTP 200 OK" "200" "$CHAL_STATUS"
assert_eq "Status respons bernilai ok" "ok" "$(echo "$CHAL_BODY" | jq -r '.status')"
assert_eq "Delivery status bernilai accepted (jujur, tidak mengklaim delivered)" "accepted" "$(echo "$CHAL_BODY" | jq -r '.delivery_status')"
assert_contains "Pesan netral dan anti-enumerasi" "diterima" "$(echo "$CHAL_BODY" | jq -r '.message')"
assert_eq "Cooldown seconds bernilai 60" "60" "$(echo "$CHAL_BODY" | jq -r '.cooldown_seconds')"

# 1.2 Verifikasi data di tabel guest_auth_challenges
CHAL_ID=$(psql "$PG_CONN" -t -A -c "SELECT id FROM guest_auth_challenges WHERE email = '${TEST_EMAIL}' ORDER BY created_at DESC LIMIT 1;")
assert_contains "Challenge ID tersimpan di database" "01" "$CHAL_ID"

# 1.3 Verifikasi atomisitas outbox: event guest.otp_dispatch harus ada di tabel outbox
OUTBOX_COUNT=$(psql "$PG_CONN" -t -A -c "SELECT COUNT(*) FROM outbox WHERE topic = 'guest.otp_dispatch' AND payload->>'email' = '${TEST_EMAIL}';")
assert_eq "Outbox event tersimpan secara atomik bersama tantangan" "1" "$OUTBOX_COUNT"

# ------------------------------------------------------------------------------
# Skenario 2: Pemrosesan Asinkron Outbox Relay (Status Transition to 'done')
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 2. Asynchronous Outbox Relay Processing ---${NC}"

# Tunggu siklus relay memproses job (OutboxInterval default 200ms - 2s)
echo -e "  ${CYAN}ℹ${NC} Menunggu OutboxRelay memproses event..."
sleep 2

OUTBOX_STATUS=$(psql "$PG_CONN" -t -A -c "SELECT status FROM outbox WHERE topic = 'guest.otp_dispatch' AND payload->>'email' = '${TEST_EMAIL}' ORDER BY id DESC LIMIT 1;")
assert_eq "Status event outbox diperbarui menjadi 'done' oleh worker relay" "done" "$OUTBOX_STATUS"

# ------------------------------------------------------------------------------
# Skenario 3: Verifikasi Payload Outbox & Idempotency Key
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 3. Payload Integrity & Per-Challenge Idempotency Key ---${NC}"

PAYLOAD_CHAL_ID=$(psql "$PG_CONN" -t -A -c "SELECT payload->>'challenge_id' FROM outbox WHERE topic = 'guest.otp_dispatch' AND payload->>'email' = '${TEST_EMAIL}' ORDER BY id DESC LIMIT 1;")
assert_eq "Payload challenge_id cocok dengan ID tantangan" "$CHAL_ID" "$PAYLOAD_CHAL_ID"

PAYLOAD_OTP=$(psql "$PG_CONN" -t -A -c "SELECT payload->>'otp_code' FROM outbox WHERE topic = 'guest.otp_dispatch' AND payload->>'email' = '${TEST_EMAIL}' ORDER BY id DESC LIMIT 1;")
assert_eq "Kode OTP 6 digit tercatat dalam payload outbox" "6" "${#PAYLOAD_OTP}"

# ------------------------------------------------------------------------------
# Skenario 4: Proteksi Kode Kedaluwarsa (Anti-Stale OTP Delivery)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 4. Anti-Stale OTP Delivery Protection ---${NC}"

STALE_EMAIL="stale.otp.$(date +%s%N)@example.id"
STALE_EXPIRES=$(date -u -d "5 minutes ago" +%Y-%m-%dT%H:%M:%SZ)

# Sisipkan manual event outbox yang sudah kedaluwarsa
psql "$PG_CONN" -c "
INSERT INTO outbox (topic, payload, status, attempts, next_retry_at, created_at)
VALUES (
    'guest.otp_dispatch',
    json_build_object('challenge_id', uuidv7(), 'email', '${STALE_EMAIL}', 'otp_code', '999111', 'expires_at', '${STALE_EXPIRES}'::timestamptz)::jsonb,
    'pending',
    0,
    now(),
    now()
);" > /dev/null

echo -e "  ${CYAN}ℹ${NC} Menunggu OutboxRelay mengevaluasi event kedaluwarsa..."
sleep 2

STALE_STATUS=$(psql "$PG_CONN" -t -A -c "SELECT status FROM outbox WHERE topic = 'guest.otp_dispatch' AND payload->>'email' = '${STALE_EMAIL}' ORDER BY id DESC LIMIT 1;")
assert_eq "Event kedaluwarsa ditandai 'done' (discarded) tanpa error" "done" "$STALE_STATUS"

# ------------------------------------------------------------------------------
# Skenario 5: Proteksi Kode Terverifikasi (Already Verified Discard)
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 5. Already Verified Challenge Protection ---${NC}"

VERIFIED_EMAIL="verified.otp.$(date +%s%N)@example.id"

# Buat tantangan yang langsung ditandai verified_at
VERIFIED_CHAL_ID=$(psql "$PG_CONN" -t -A -c "
INSERT INTO guest_auth_challenges (email, code_hash, attempts, max_attempts, expires_at, verified_at)
VALUES ('${VERIFIED_EMAIL}', 'hash_dummy', 0, 3, NOW() + INTERVAL '10 minutes', NOW() - INTERVAL '1 minute')
RETURNING id;
")

# Masukkan event pending yang merujuk ke tantangan yang sudah verified
psql "$PG_CONN" -c "
INSERT INTO outbox (topic, payload, status, attempts, next_retry_at, created_at)
VALUES (
    'guest.otp_dispatch',
    json_build_object('challenge_id', '${VERIFIED_CHAL_ID}', 'email', '${VERIFIED_EMAIL}', 'otp_code', '777888', 'expires_at', (NOW() + INTERVAL '10 minutes')::text)::jsonb,
    'pending',
    0,
    now(),
    now()
);" > /dev/null

echo -e "  ${CYAN}ℹ${NC} Menunggu OutboxRelay mengevaluasi event terverifikasi..."
sleep 2

VERIFIED_STATUS=$(psql "$PG_CONN" -t -A -c "SELECT status FROM outbox WHERE topic = 'guest.otp_dispatch' AND payload->>'email' = '${VERIFIED_EMAIL}' ORDER BY id DESC LIMIT 1;")
assert_eq "Event tantangan terverifikasi ditandai 'done' tanpa kirim ulang" "done" "$VERIFIED_STATUS"

# ------------------------------------------------------------------------------
# Skenario 6: Verifikasi Login Menggunakan OTP dari Outbox
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}--- 6. End-to-End Verification with Outbox OTP ---${NC}"

VERIFY_RES=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/auth/guest/verify" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${TEST_EMAIL}\",\"code\":\"${PAYLOAD_OTP}\"}")
VERIFY_STATUS=$(echo "$VERIFY_RES" | tail -n1)
VERIFY_BODY=$(echo "$VERIFY_RES" | head -n-1)

assert_eq "Verifikasi dengan kode OTP dari outbox berhasil (HTTP 200 OK)" "200" "$VERIFY_STATUS"
assert_eq "Email terverifikasi cocok" "${TEST_EMAIL}" "$(echo "$VERIFY_BODY" | jq -r '.email')"
assert_contains "Session token berhasil diterbitkan" "gst_sess_" "$(echo "$VERIFY_BODY" | jq -r '.token')"

# ------------------------------------------------------------------------------
# Hasil Akhir
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}=================================================================${NC}"
echo -e "Total Asersi: ${TOTAL}"
echo -e "Lolos (Pass): ${GREEN}${PASSED}${NC}"
echo -e "Gagal (Fail): ${RED}${FAILED}${NC}"
echo -e "${BLUE}=================================================================${NC}"

if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}E2E TEST GAGAL! Periksa log kesalahan di atas.${NC}"
    exit 1
fi

echo -e "${GREEN}SELURUH PENGUJIAN E2E BE-R17 BERHASIL DENGAN SEMPURNA! (100% PASS)${NC}\n"
