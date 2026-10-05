#!/usr/bin/env bash
# ==============================================================================
# End-to-End (E2E) Test Suite: Comprehensive Multi-Role RBAC Matrix
# Properti: Hotel Pulang ke Uttara (Yogyakarta)
# Skenario: Uji Alur Penuh 6 Role (guest, receptionist, housekeeping, revenue_mgr, finance, gm_admin)
# ==============================================================================

set -euo pipefail

BASE_URL="${BASE_URL:-${API_BASE_URL:-https://hotel.fied.space}}"
STAFF_PASSWORD="${STAFF_PASSWORD:-Password12345!}"

TOTAL=0
PASSED=0
FAILED=0

# Colors for terminal output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  PULANG KE UTTARA — MULTI-ROLE END-TO-END VERIFICATION MATRIX                ${NC}"
echo -e "${BLUE}  Target: ${BASE_URL}                                                         ${NC}"
echo -e "${BLUE}==============================================================================${NC}\n"

assert_status() {
    local test_name="$1"
    local expected="$2"
    local actual="$3"
    local response_body="$4"

    TOTAL=$((TOTAL + 1))
    if [ "$actual" -eq "$expected" ]; then
        echo -e "  [${GREEN}PASS${NC}] ${test_name} (HTTP ${actual})"
        PASSED=$((PASSED + 1))
    else
        echo -e "  [${RED}FAIL${NC}] ${test_name} — Expected HTTP ${expected}, Got ${actual}"
        echo -e "         Body: ${response_body}"
        FAILED=$((FAILED + 1))
    fi
}

login_staff() {
    local username="$1"
    local resp
    resp=$(curl -s -X POST "${BASE_URL}/api/v1/auth/staff/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"${username}\",\"password\":\"${STAFF_PASSWORD}\"}")
    local tok
    tok=$(echo "$resp" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
    if [ -z "$tok" ]; then
        echo -e "${RED}Error: Login failed for ${username}${NC}" >&2
        echo "Response: $resp" >&2
        exit 1
    fi
    echo "$tok"
}

# ------------------------------------------------------------------------------
# 0. Otentikasi Seluruh Role Staf
# ------------------------------------------------------------------------------
echo -e "${YELLOW}>> 0. Mengautentikasi Seluruh Akun Staf (BE-R01) <<${NC}"
T_RECEPTIONIST=$(login_staff fo_receptionist)
echo -e "  [${GREEN}OK${NC}] Receptionist Token: ${T_RECEPTIONIST:0:15}..."
T_HOUSEKEEPING=$(login_staff hk_lead)
echo -e "  [${GREEN}OK${NC}] Housekeeping Token: ${T_HOUSEKEEPING:0:15}..."
T_REVENUE_MGR=$(login_staff rev_mgr)
echo -e "  [${GREEN}OK${NC}] Revenue Manager Token: ${T_REVENUE_MGR:0:15}..."
T_FINANCE=$(login_staff fin_officer)
echo -e "  [${GREEN}OK${NC}] Finance Officer Token: ${T_FINANCE:0:15}..."
T_GM_ADMIN=$(login_staff admin_gm)
echo -e "  [${GREEN}OK${NC}] General Manager Token: ${T_GM_ADMIN:0:15}..."

# ------------------------------------------------------------------------------
# 1. Role: GUEST (Tamu Publik)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}==============================================================================${NC}"
echo -e "${CYAN}  1. ROLE: GUEST (Tamu Publik)                                               ${NC}"
echo -e "${CYAN}==============================================================================${NC}"

# 1.1 Discovery Katalog Kamar
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" "${BASE_URL}/api/v1/catalog/rooms")
assert_status "Guest: Menjelajah katalog kamar publik" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 1.2 Multi-night Search
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    "${BASE_URL}/api/v1/search?check_in=2026-10-18&check_out=2026-10-20&rooms=1&adults=2")
assert_status "Guest: Pencarian ketersediaan kamar multi-malam" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 1.3 Pembuatan Locked Quote 15-Menit
QUOTE_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d '{
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "rate_plan_code": "room_only",
        "check_in": "2026-10-18",
        "check_out": "2026-10-20",
        "num_rooms": 1,
        "num_guests": 2
    }')
QUOTE_ID=$(echo "$QUOTE_RESP" | sed -n 's/.*"quote_id":"\([^"]*\)".*/\1/p')
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
    -H "Content-Type: application/json" \
    -d '{
        "room_type_id": "01900000-0000-7000-8000-000000000001",
        "rate_plan_code": "room_only",
        "check_in": "2026-10-18",
        "check_out": "2026-10-20",
        "num_rooms": 1,
        "num_guests": 2
    }')
assert_status "Guest: Mengunci harga (15-min locked quote)" 200 "$HTTP_CODE" "$QUOTE_RESP"

# 1.4 Pembuatan Booking Hold
BOOKING_PAYLOAD="{
    \"quote_id\": \"${QUOTE_ID}\",
    \"terms_accepted\": true,
    \"privacy_accepted\": true,
    \"room_type_id\": \"01900000-0000-7000-8000-000000000001\",
    \"check_in\": \"2026-10-18\",
    \"check_out\": \"2026-10-20\",
    \"num_rooms\": 1,
    \"num_guests\": 2,
    \"guest_name\": \"Andi Wijaya\",
    \"guest_email\": \"andi.wijaya@example.com\",
    \"guest_phone\": \"+6281234567890\",
    \"estimated_arrival_time\": \"14:00\",
    \"special_requests\": \"Lantai atas dan kamar bebas asap rokok\"
}"
BOOKING_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/bookings" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ik-role-guest-$(date +%s%N)" \
    -d "$BOOKING_PAYLOAD")
GUEST_BID=$(echo "$BOOKING_RESP" | sed -n 's/.*"booking":{"id":"\([^"]*\)".*/\1/p')
GUEST_TOKEN=$(echo "$BOOKING_RESP" | sed -n 's/.*"guest_access_token":"\([^"]*\)".*/\1/p')
HTTP_CODE=$(echo "$BOOKING_RESP" | grep -q '"status":"pending"' && echo 201 || echo 500)
assert_status "Guest: Reservasi kamar hold baru dibuat" 201 "$HTTP_CODE" "$BOOKING_RESP"

# 1.5 Masking PII pada Tampilan Publik (UU PDP)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" "${BASE_URL}/api/v1/bookings/${GUEST_BID}")
assert_status "Guest: Query tanpa otentikasi membatasi PII (PublicDTO)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 1.6 Otorisasi Pemilik Reservasi via X-Guest-Token
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "X-Guest-Token: ${GUEST_TOKEN}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}")
assert_status "Guest: Pemilik dengan X-Guest-Token memperoleh rincian lengkap" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 1.7 Batasan Negatif Guest (Forbidden Checks)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" -X POST "${BASE_URL}/api/v1/bookings/${GUEST_BID}/check-in")
assert_status "Guest NEGATIF: Dilarang check-in mandiri (403 Forbidden)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" -X GET "${BASE_URL}/api/v1/front-desk/daily-roster")
assert_status "Guest NEGATIF: Dilarang mengakses Daily Roster Meja Depan (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" -X GET "${BASE_URL}/api/v1/housekeeping/rooms")
assert_status "Guest NEGATIF: Dilarang mengakses Room Board Housekeeping (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# 2. Role: RECEPTIONIST (Staf Meja Depan / Front Desk)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}==============================================================================${NC}"
echo -e "${CYAN}  2. ROLE: RECEPTIONIST (Staf Meja Depan)                                    ${NC}"
echo -e "${CYAN}==============================================================================${NC}"

# 2.1 Akses Daily Roster
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    "${BASE_URL}/api/v1/front-desk/daily-roster")
assert_status "Receptionist: Mengakses Daily Operations Roster (95 kamar)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 2.2 Pencatatan Shift Handover Logbook
NOTE_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/front-desk/handover-notes" \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    -H "Content-Type: application/json" \
    -d '{
        "shift": "afternoon",
        "cash_float_minor": 2000000,
        "pending_issues": "Kamar 301 komplain shower; teknisi dijadwalkan 16:30",
        "vip_guest_notes": "Tamu VIP Ibu Maya check-in malam ini"
    }')
HTTP_CODE=$(echo "$NOTE_RESP" | grep -q '"status":"ok"' && echo 201 || echo 500)
assert_status "Receptionist: Mencatat Shift Handover Note meja depan" 201 "$HTTP_CODE" "$NOTE_RESP"

# 2.3 Pelunasan & Check-in Tamu
curl -s -o /dev/null -X POST "${BASE_URL}/fake-pay/ref-pay?booking_id=${GUEST_BID}"
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}/check-in")
assert_status "Receptionist: Melakukan check-in tamu terkonfirmasi" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 2.4 Perpanjangan Masa Tinggal (Stay Extension)
EXT_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/bookings/${GUEST_BID}/extend-stay" \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    -H "Content-Type: application/json" \
    -d '{"additional_nights": 1}')
HTTP_CODE=$(echo "$EXT_RESP" | grep -q '"status":"ok"' && echo 200 || echo 500)
assert_status "Receptionist: Memperpanjang masa tinggal (+1 malam)" 200 "$HTTP_CODE" "$EXT_RESP"

# 2.5 Unduh Official PDF Voucher
HTTP_CODE=$(curl -s -o /tmp/voucher.pdf -w "%{http_code}" \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}/voucher.pdf")
assert_status "Receptionist: Mengunduh Confirmation Voucher PDF" 200 "$HTTP_CODE" ""

# 2.6 Batasan Negatif Receptionist (Forbidden Checks)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X GET \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}/invoice.pdf")
assert_status "Receptionist NEGATIF: Dilarang mengunduh Faktur Pajak PBJT (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X PUT \
    -H "Authorization: Bearer ${T_RECEPTIONIST}" \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}' \
    "${BASE_URL}/api/v1/admin/feature-flags/ff_official_pdf_voucher")
assert_status "Receptionist NEGATIF: Dilarang memodifikasi Feature Flags (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# 3. Role: HOUSEKEEPING (Tata Graha)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}==============================================================================${NC}"
echo -e "${CYAN}  3. ROLE: HOUSEKEEPING (Tata Graha)                                         ${NC}"
echo -e "${CYAN}==============================================================================${NC}"

# 3.1 Housekeeping Room Board
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
    "${BASE_URL}/api/v1/housekeeping/rooms?floor=2")
assert_status "Housekeeping: Membaca status kebersihan kamar (Floor 2 Board)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 3.2 Transisi Status Kebersihan (cleaning -> clean)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X PUT \
    -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
    -H "Content-Type: application/json" \
    -d '{"to_status":"cleaning","notes":"Attendant mulai pembersihan"}' \
    "${BASE_URL}/api/v1/housekeeping/rooms/204/status")
assert_status "Housekeeping: Transisi dirty -> cleaning kamar 204" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X PUT \
    -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
    -H "Content-Type: application/json" \
    -d '{"to_status":"clean","notes":"Linen dan handuk baru terpasang"}' \
    "${BASE_URL}/api/v1/housekeeping/rooms/204/status")
assert_status "Housekeeping: Transisi cleaning -> clean kamar 204" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 3.3 Batasan Negatif Housekeeping (Forbidden Checks)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}/check-in")
assert_status "Housekeeping NEGATIF: Dilarang memproses check-in tamu (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X GET \
    -H "Authorization: Bearer ${T_HOUSEKEEPING}" \
    "${BASE_URL}/api/v1/front-desk/handover-notes")
assert_status "Housekeeping NEGATIF: Dilarang membaca Handover Notes internal Meja Depan (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# 4. Role: REVENUE_MGR (Manajer Pendapatan & Tarif)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}==============================================================================${NC}"
echo -e "${CYAN}  4. ROLE: REVENUE_MGR (Manajer Pendapatan & Yield)                          ${NC}"
echo -e "${CYAN}==============================================================================${NC}"

# 4.1 Pembuatan Varian Kamar Baru di Katalog
CREATE_ROOM_RESP=$(curl -s -X POST "${BASE_URL}/api/v1/catalog/rooms" \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    -H "Content-Type: application/json" \
    -d '{
        "code": "suite-penthouse",
        "name": "Uttara Penthouse Suite",
        "family_name": "Penthouse",
        "bed_type": "1 Super King Bed",
        "room_size_sqm": 120,
        "max_capacity": 4,
        "max_adults": 2,
        "max_children": 2,
        "base_price_minor": 3500000,
        "description": "Penthouse mewah dengan pemandangan Gunung Merapi.",
        "amenities": ["Jacuzzi", "Private Balcony", "High-speed Wi-Fi"],
        "photos": [{"url": "https://hotel.fied.space/photos/penthouse.jpg", "alt": "Penthouse"}]
    }')
NEW_ROOM_ID=$(echo "$CREATE_ROOM_RESP" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
HTTP_CODE=$(echo "$CREATE_ROOM_RESP" | grep -q '"id"' && echo 201 || echo 500)
assert_status "Revenue Mgr: Membuat varian kamar baru di katalog" 201 "$HTTP_CODE" "$CREATE_ROOM_RESP"

# 4.2 Inspeksi Kalender Tarif & Allotment
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    "${BASE_URL}/api/v1/revenue/calendar?start_date=2026-10-18&end_date=2026-10-25")
assert_status "Revenue Mgr: Menginspeksi kalender tarif (Revenue Calendar)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 4.3 Penerapan Bulk Stop-Sell Override
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X PUT \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    -H "Content-Type: application/json" \
    -d '{
        "room_type_ids": ["01900000-0000-7000-8000-000000000001"],
        "start_date": "2026-10-24",
        "end_date": "2026-10-24",
        "rate_plan_code": "room_only",
        "is_stop_sell": true
    }' \
    "${BASE_URL}/api/v1/revenue/calendar/bulk")
assert_status "Revenue Mgr: Menetapkan batasan Stop-Sell tanggal 2026-10-24" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 4.4 Batasan Negatif Revenue Manager (Forbidden Checks)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X DELETE \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    "${BASE_URL}/api/v1/catalog/rooms/${NEW_ROOM_ID}")
assert_status "Revenue Mgr NEGATIF: Dilarang menghapus varian kamar (403, GM only)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_REVENUE_MGR}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}/check-in")
assert_status "Revenue Mgr NEGATIF: Dilarang check-in tamu (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# 5. Role: FINANCE (Staf Akuntansi & Keuangan)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}==============================================================================${NC}"
echo -e "${CYAN}  5. ROLE: FINANCE (Keuangan & Perpajakan Daerah)                            ${NC}"
echo -e "${CYAN}==============================================================================${NC}"

# 5.1 Akses Faktur Pajak Daerah PBJT Sleman (PDF)
HTTP_CODE=$(curl -s -o /tmp/invoice.pdf -w "%{http_code}" \
    -H "Authorization: Bearer ${T_FINANCE}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}/invoice.pdf")
assert_status "Finance: Mengunduh Faktur Pajak Daerah PBJT Sleman (PDF)" 200 "$HTTP_CODE" ""

# 5.2 Akses Ringkasan Rekonsiliasi & Sengketa Keuangan
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Authorization: Bearer ${T_FINANCE}" \
    "${BASE_URL}/api/v1/finance/reconciliations")
assert_status "Finance: Membaca ringkasan rekonsiliasi keuangan (Finance Reconciliations)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 5.3 Batasan Negatif Finance (Forbidden Checks)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Authorization: Bearer ${T_FINANCE}" \
    "${BASE_URL}/api/v1/front-desk/daily-roster")
assert_status "Finance NEGATIF: Dilarang mengakses Daily Operations Roster Meja Depan (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X POST \
    -H "Authorization: Bearer ${T_FINANCE}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}/check-in")
assert_status "Finance NEGATIF: Dilarang melakukan check-in tamu (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X PUT \
    -H "Authorization: Bearer ${T_FINANCE}" \
    -H "Content-Type: application/json" \
    -d '{"to_status":"clean"}' \
    "${BASE_URL}/api/v1/housekeeping/rooms/204/status")
assert_status "Finance NEGATIF: Dilarang mengubah status kebersihan kamar (403)" 403 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# 6. Role: GM_ADMIN (General Manager / Super Administrator)
# ------------------------------------------------------------------------------
echo -e "\n${CYAN}==============================================================================${NC}"
echo -e "${CYAN}  6. ROLE: GM_ADMIN (General Manager / Super Admin)                          ${NC}"
echo -e "${CYAN}==============================================================================${NC}"

# 6.1 Inspeksi Feature Flags
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Authorization: Bearer ${T_GM_ADMIN}" \
    "${BASE_URL}/api/v1/admin/feature-flags")
assert_status "GM Admin: Menginspeksi status seluruh Feature Flags" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 6.2 Penghapusan Varian Kamar di Katalog (Hanya GM yang berwenang)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -X DELETE \
    -H "Authorization: Bearer ${T_GM_ADMIN}" \
    "${BASE_URL}/api/v1/catalog/rooms/${NEW_ROOM_ID}")
assert_status "GM Admin: Menghapus varian kamar dari katalog (Otoritas Penuh)" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 6.3 Verifikasi Kamar yang Dihapus Menghasilkan 404
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" "${BASE_URL}/api/v1/catalog/rooms/${NEW_ROOM_ID}")
assert_status "GM Admin: Verifikasi kamar terhapus mengembalikan 404 Not Found" 404 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# 6.4 Inspeksi Reservasi Apapun (Super-Admin Wildcard)
HTTP_CODE=$(curl -s -o /tmp/e2e_res.json -w "%{http_code}" \
    -H "Authorization: Bearer ${T_GM_ADMIN}" \
    "${BASE_URL}/api/v1/bookings/${GUEST_BID}")
assert_status "GM Admin: Menginspeksi reservasi manapun secara global" 200 "$HTTP_CODE" "$(cat /tmp/e2e_res.json)"

# ------------------------------------------------------------------------------
# Ringkasan Akhir
# ------------------------------------------------------------------------------
echo -e "\n${BLUE}==============================================================================${NC}"
echo -e "${BLUE}  RINGKASAN MATRIX E2E MULTI-ROLE SELESAI                                    ${NC}"
echo -e "${BLUE}==============================================================================${NC}"
echo -e "Total Asersi   : ${TOTAL}"
echo -e "Berhasil (Pass): ${GREEN}${PASSED}${NC}"
echo -e "Gagal (Fail)   : ${RED}${FAILED}${NC}"

if [ "$FAILED" -gt 0 ]; then
    echo -e "\n${RED}✗ Pengujian E2E Multi-Role GAGAL!${NC}"
    exit 1
fi

echo -e "\n${GREEN}★★★ SELURUH 6 ROLE LULUS 100% PENGUJIAN OTORISASI & KEAMANAN RBAC ★★★${NC}"
exit 0
