#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Last-Room Hospitality Safeguards & 1-Click Upgrade Resolver
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Skenario Pengujian:
# 1. In-Process Go E2E Suite (E2E-79: Dynamic Hold & LRDA, E2E-80: 1-Click Upgrade & RBAC)
# 2. Ephemeral Live Server Lifecycle Boot (Port 28094)
# 3. Quota Conflict Quarantine: OTA Overbooking returns 409 Conflict
# 4. Receptionist Access: Receptionist lists channel sync issues (200 OK)
# 5. RBAC Defense: Housekeeping forbidden from resolving issue (403 Forbidden)
# 6. Validation Defense: Complimentary upgrade without target room type returns 400 Bad Request
# 7. Validation Defense: Invalid resolution action returns 400 Bad Request
# 8. Meja Depan 1-Click Upgrade: Receptionist executes COMPLIMENTARY_UPGRADE (200 OK)
# 9. Concurrency Defense: Double-resolution of already resolved issue returns 409 Conflict
# 10. Alternative Resolution: Staf executes REJECT_AND_CANCEL on quarantined issue (200 OK)
# 11. Alternative Resolution: GM Admin executes FORCE_OVERBOOK_CONFIRMED (200 OK)
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
    if echo "$haystack" | grep -q "$needle"; then
        echo -e "  ${GREEN}✓${NC} ${desc} (Contains: '${needle}')"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${RED}✗${NC} ${desc} — Needle '${needle}' not found in:\n${haystack}"
        FAILED=$((FAILED + 1))
    fi
}

echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  E2E TEST: LAST-ROOM SAFEGUARDS & COMPLIMENTARY UPGRADE RESOLVER            ${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

# ------------------------------------------------------------------------------
# 1. Run Automated In-Process Go E2E Suite (E2E-79 & E2E-80)
# ------------------------------------------------------------------------------
echo -e "${CYAN}1. Menjalankan Go In-Process E2E Suite (E2E-79 & E2E-80)...${NC}"
if (cd "${REPO_ROOT}" && go test -v ./testing/e2e/script -run 'TestEndToEndHotelBookingRBACLifecycle/(E2E-79|E2E-80)'); then
    echo -e "  ${GREEN}✓${NC} Go E2E Suite (E2E-79 & E2E-80) passed successfully"
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
TMP_DIR="$(mktemp -d /tmp/e2e_last_room_XXXXXX)"
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
    EPHEMERAL_PORT=28094
    BASE_URL="http://127.0.0.1:${EPHEMERAL_PORT}"
    echo -e "\n${CYAN}2. Memulai Ephemeral Live Server pada port ${EPHEMERAL_PORT}...${NC}"
    (cd "${REPO_ROOT}" && RUN_EPHEMERAL_E2E_SERVER_PORT="${EPHEMERAL_PORT}" go test -v ./testing/e2e/script -run TestEphemeralServerRunner > "$TMP_DIR/ephemeral.log" 2>&1) &
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
        echo -e "${RED}Ephemeral server gagal aktif! Log:${NC}"
        cat "$TMP_DIR/ephemeral.log" || true
        exit 1
    fi
    echo -e "  ${GREEN}✓${NC} Ephemeral live server berhasil aktif di ${BASE_URL}"
fi

# ------------------------------------------------------------------------------
# 3. Quota Conflict Ingestion: OTA Overbooking Webhook returns 409 Conflict
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}3. Pengujian Ingestion OTA Webhook Overbooking (409 Conflict)...${NC}"
OVERBOOK_PAYLOAD='{
    "provider": "AGODA",
    "event_id": "evt_agoda_overbook_999",
    "event_type": "reservation_created",
    "external_reference": "AGD-OVR-999",
    "room_type_id": "01900000-0000-7000-8000-000000000001",
    "check_in": "2026-10-10",
    "check_out": "2026-10-12",
    "rooms": 50,
    "guest_name": "Overbook Guest Agoda",
    "guest_email": "overbook@agoda.example.com",
    "guest_phone": "+6281199887766",
    "total_payout_idr": 55000000
}'
SECRET="agd_secret_webhook_signature_key_2026"
HMAC_SIG=$(echo -n "${OVERBOOK_PAYLOAD}" | openssl dgst -sha256 -hmac "${SECRET}" | awk '{print $NF}')

RESP_OVR=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/channel-events" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: AGODA" \
    -H "X-Channel-Signature: ${HMAC_SIG}" \
    -d "${OVERBOOK_PAYLOAD}")
OVR_STATUS=$(echo "$RESP_OVR" | tail -n1)
OVR_BODY=$(echo "$RESP_OVR" | head -n -1)
assert_eq "OTA overbooking ditolak 409 Conflict" "409" "$OVR_STATUS"
assert_contains "Respons memuat kode ALLOTMENT_EXHAUSTED" "ALLOTMENT_EXHAUSTED" "$OVR_BODY"

# ------------------------------------------------------------------------------
# 4. Receptionist Access: Receptionist lists channel sync issues (200 OK)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}4. Pengujian Otorisasi Meja Depan Membaca Isu Sinkronisasi (200 OK)...${NC}"
RESP_ISSUES=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/api/v1/staff/channel-sync-issues" \
    -H "Authorization: Bearer receptionist")
ISSUES_STATUS=$(echo "$RESP_ISSUES" | tail -n1)
ISSUES_BODY=$(echo "$RESP_ISSUES" | head -n -1)
assert_eq "Receptionist berhasil membaca daftar isu (200 OK)" "200" "$ISSUES_STATUS"
assert_contains "Daftar memuat external_reference AGD-OVR-999" "AGD-OVR-999" "$ISSUES_BODY"

TARGET_ISSUE_ID=$(echo "$ISSUES_BODY" | jq -r '.issues[] | select(.external_reference=="AGD-OVR-999") | .id')

# ------------------------------------------------------------------------------
# 5. RBAC Defense: Housekeeping forbidden from resolving issue (403 Forbidden)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}5. Pengujian Keamanan RBAC Resolusi Isu oleh Housekeeping (403 Forbidden)...${NC}"
UPGRADE_PAYLOAD='{
    "action": "COMPLIMENTARY_UPGRADE",
    "target_room_type_id": "01900000-0000-7000-8000-000000000002",
    "notes": "Upgrade gratis ke Deluxe Room oleh Meja Depan"
}'
RESP_HK=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/staff/channel-sync-issues/${TARGET_ISSUE_ID}/resolve" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer housekeeping" \
    -d "${UPGRADE_PAYLOAD}")
HK_STATUS=$(echo "$RESP_HK" | tail -n1)
assert_eq "Housekeeping dilarang menyelesaikan isu (403 Forbidden)" "403" "$HK_STATUS"

# ------------------------------------------------------------------------------
# 6. Validation Defense: Missing target room type on COMPLIMENTARY_UPGRADE (400 Bad Request)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}6. Pengujian Validasi Target Room Type Kosong pada Upgrade (400 Bad Request)...${NC}"
MISSING_TARGET_PAYLOAD='{
    "action": "COMPLIMENTARY_UPGRADE"
}'
RESP_MISSING=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/staff/channel-sync-issues/${TARGET_ISSUE_ID}/resolve" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer receptionist" \
    -d "${MISSING_TARGET_PAYLOAD}")
MISSING_STATUS=$(echo "$RESP_MISSING" | tail -n1)
MISSING_BODY=$(echo "$RESP_MISSING" | head -n -1)
assert_eq "Upgrade tanpa target_room_type_id ditolak 400 Bad Request" "400" "$MISSING_STATUS"
assert_contains "Respons memuat kode MISSING_TARGET_ROOM_TYPE" "MISSING_TARGET_ROOM_TYPE" "$MISSING_BODY"

# ------------------------------------------------------------------------------
# 7. Validation Defense: Invalid resolution action returns 400 Bad Request
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}7. Pengujian Validasi Aksi Resolusi Tidak Dikenal (400 Bad Request)...${NC}"
INVALID_ACTION_PAYLOAD='{
    "action": "ARBITRARY_ACTION_UNKNOWN"
}'
RESP_INVAL=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/staff/channel-sync-issues/${TARGET_ISSUE_ID}/resolve" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer receptionist" \
    -d "${INVALID_ACTION_PAYLOAD}")
INVAL_STATUS=$(echo "$RESP_INVAL" | tail -n1)
INVAL_BODY=$(echo "$RESP_INVAL" | head -n -1)
assert_eq "Aksi tidak dikenal ditolak 400 Bad Request" "400" "$INVAL_STATUS"
assert_contains "Respons memuat kode INVALID_ACTION" "INVALID_ACTION" "$INVAL_BODY"

# ------------------------------------------------------------------------------
# 8. Meja Depan 1-Click Upgrade: Receptionist executes COMPLIMENTARY_UPGRADE (200 OK)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}8. Pengujian Eksekusi 1-Click Complimentary Upgrade oleh Receptionist (200 OK)...${NC}"
RESP_UPG=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/staff/channel-sync-issues/${TARGET_ISSUE_ID}/resolve" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer receptionist" \
    -d "${UPGRADE_PAYLOAD}")
UPG_STATUS=$(echo "$RESP_UPG" | tail -n1)
UPG_BODY=$(echo "$RESP_UPG" | head -n -1)
assert_eq "Complimentary upgrade berhasil (200 OK)" "200" "$UPG_STATUS"
assert_contains "Status isu berubah menjadi RESOLVED" "RESOLVED" "$UPG_BODY"
assert_contains "Target room type tercatat di issue" "01900000-0000-7000-8000-000000000002" "$UPG_BODY"

# ------------------------------------------------------------------------------
# 9. Concurrency Defense: Double-resolution of already resolved issue returns 409 Conflict
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}9. Pengujian Pencegahan Double-Resolution pada Isu yang Sudah Selesai (409 Conflict)...${NC}"
RESP_DOUBLE=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/staff/channel-sync-issues/${TARGET_ISSUE_ID}/resolve" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer receptionist" \
    -d "${UPGRADE_PAYLOAD}")
DOUBLE_STATUS=$(echo "$RESP_DOUBLE" | tail -n1)
DOUBLE_BODY=$(echo "$RESP_DOUBLE" | head -n -1)
assert_eq "Resolusi kedua ditolak 409 Conflict" "409" "$DOUBLE_STATUS"
assert_contains "Respons memuat kode ALREADY_RESOLVED" "ALREADY_RESOLVED" "$DOUBLE_BODY"

# ------------------------------------------------------------------------------
# 10. Alternative Resolution: Reject and Cancel on another quarantined issue (200 OK)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}10. Pengujian Aksi Alternatif REJECT_AND_CANCEL...${NC}"
REJECT_EVENT='{
    "provider": "TRAVELOKA",
    "event_id": "evt_trv_reject_001",
    "event_type": "reservation_created",
    "external_reference": "TRV-REJ-001",
    "room_type_id": "01900000-0000-7000-8000-000000000001",
    "check_in": "2026-10-10",
    "check_out": "2026-10-12",
    "rooms": 40,
    "guest_name": "Reject Guest",
    "guest_email": "reject@example.com",
    "guest_phone": "+6281100001111",
    "total_payout_idr": 40000000
}'
TRV_SECRET="trv_secret_webhook_signature_key_2026"
TRV_SIG=$(echo -n "${REJECT_EVENT}" | openssl dgst -sha256 -hmac "${TRV_SECRET}" | awk '{print $NF}')
curl -s -X POST "${BASE_URL}/api/v1/channel-events" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: TRAVELOKA" \
    -H "X-Channel-Signature: ${TRV_SIG}" \
    -d "${REJECT_EVENT}" > /dev/null

REJ_ISSUES=$(curl -s -X GET "${BASE_URL}/api/v1/staff/channel-sync-issues" -H "Authorization: Bearer receptionist")
REJ_ISSUE_ID=$(echo "$REJ_ISSUES" | jq -r '.issues[] | select(.external_reference=="TRV-REJ-001") | .id')

REJ_PAYLOAD='{
    "action": "REJECT_AND_CANCEL",
    "notes": "Hotel penuh, tamu dialihkan ke partner hotel"
}'
RESP_REJ=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/staff/channel-sync-issues/${REJ_ISSUE_ID}/resolve" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer receptionist" \
    -d "${REJ_PAYLOAD}")
REJ_STATUS=$(echo "$RESP_REJ" | tail -n1)
REJ_BODY=$(echo "$RESP_REJ" | head -n -1)
assert_eq "Reject and cancel berhasil (200 OK)" "200" "$REJ_STATUS"
assert_contains "Status isu berubah menjadi REJECTED" "REJECTED" "$REJ_BODY"

# ------------------------------------------------------------------------------
# 11. Alternative Resolution: GM Admin executes FORCE_OVERBOOK_CONFIRMED (200 OK)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}11. Pengujian Aksi FORCE_OVERBOOK_CONFIRMED oleh GM Admin...${NC}"
GM_EVENT='{
    "provider": "AGODA",
    "event_id": "evt_agoda_gm_001",
    "event_type": "reservation_created",
    "external_reference": "AGD-GM-001",
    "room_type_id": "01900000-0000-7000-8000-000000000001",
    "check_in": "2026-10-10",
    "check_out": "2026-10-12",
    "rooms": 30,
    "guest_name": "VIP Guest GM Approved",
    "guest_email": "vip@agoda.example.com",
    "guest_phone": "+6281177778888",
    "total_payout_idr": 30000000
}'
GM_SIG=$(echo -n "${GM_EVENT}" | openssl dgst -sha256 -hmac "${SECRET}" | awk '{print $NF}')
curl -s -X POST "${BASE_URL}/api/v1/channel-events" \
    -H "Content-Type: application/json" \
    -H "X-Channel-Provider: AGODA" \
    -H "X-Channel-Signature: ${GM_SIG}" \
    -d "${GM_EVENT}" > /dev/null

GM_ISSUES=$(curl -s -X GET "${BASE_URL}/api/v1/staff/channel-sync-issues" -H "Authorization: Bearer gm_admin")
GM_ISSUE_ID=$(echo "$GM_ISSUES" | jq -r '.issues[] | select(.external_reference=="AGD-GM-001") | .id')

GM_PAYLOAD='{
    "action": "FORCE_OVERBOOK_CONFIRMED",
    "notes": "Persetujuan darurat GM untuk membuka kamar manajemen 501"
}'
RESP_GM=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/api/v1/staff/channel-sync-issues/${GM_ISSUE_ID}/resolve" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer gm_admin" \
    -d "${GM_PAYLOAD}")
GM_STATUS=$(echo "$RESP_GM" | tail -n1)
GM_BODY=$(echo "$RESP_GM" | head -n -1)
assert_eq "Force overbook confirmed berhasil (200 OK)" "200" "$GM_STATUS"
assert_contains "Status isu berubah menjadi OVERBOOKED_OVERRIDDEN" "OVERBOOKED_OVERRIDDEN" "$GM_BODY"

# ------------------------------------------------------------------------------
# Summary
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  RINGKASAN HASIL PENGUJIAN E2E: LAST-ROOM SAFEGUARDS                         ${NC}"
echo -e "${BLUE}==============================================================================${NC}"
echo -e "Total Pengujian: ${TOTAL}"
echo -e "Lulus (Passed) : ${GREEN}${PASSED}${NC}"
echo -e "Gagal (Failed) : ${RED}${FAILED}${NC}"

if [ "$FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}★★★ SELURUH PENGUJIAN E2E BERHASIL (100% PASS) ★★★${NC}\n"
    exit 0
else
    echo -e "\n${RED}✗ TERDAPAT PENGUJIAN YANG GAGAL (${FAILED}/${TOTAL})${NC}\n"
    exit 1
fi
