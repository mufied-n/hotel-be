#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Transport Layer Modernization (Gin, JSON v2, Validator v10)
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Menguji:
# 1. Gin Router Healthz, Ready, and Header Injection (X-Request-Id)
# 2. Gin URL Param Parsing (:id) on Catalog Room Endpoint
# 3. JSON v2 Security Rejection (Duplicate Object Keys)
# 4. Validator v10 Schema Validation & RFC 7807 Problem Details
# 5. Casbin RBAC Fail-Closed Enforcement on Gin Engine
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:18080}"
source "$(dirname "${BASH_SOURCE[0]}")/lib_staff_login.sh"
load_staff_tokens
GM_TOKEN="${T_GM_ADMIN}"
RECEPTION_TOKEN="${T_RECEPTIONIST}"

echo "=== [E2E] Menjalankan Pengujian Modernisasi Transport Layer ==="
echo "Target Base URL: ${BASE_URL}"

# 1. Healthcheck & Request-Id Injection
echo "--- 1. Gin Engine Healthcheck & X-Request-Id Header ---"
HEALTH_HEADER=$(curl -sS -I "${BASE_URL}/healthz")
if echo "${HEALTH_HEADER}" | grep -iq "x-request-id"; then
  echo "✓ Gin middleware berhasil menginjeksikan header X-Request-Id"
else
  echo "✗ Header X-Request-Id tidak ditemukan pada respon /healthz"
  exit 1
fi

HEALTH_STATUS=$(echo "${HEALTH_HEADER}" | head -n 1 | awk '{print $2}')
if [ "${HEALTH_STATUS}" -eq 200 ]; then
  echo "✓ Healthcheck 200 OK"
else
  echo "✗ Healthcheck mengembalikan status: ${HEALTH_STATUS}"
  exit 1
fi

# 2. Ready Check
echo "--- 2. Gin Engine Ready Check ---"
READY_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" "${BASE_URL}/ready")
if [ "${READY_STATUS}" -eq 200 ]; then
  echo "✓ Ready status 200 OK"
else
  echo "✗ Ready status gagal: HTTP ${READY_STATUS}"
  exit 1
fi

# 3. Catalog Room Parameter Extraction (:id)
echo "--- 3. Gin Path Parameter Extraction (:id) on Catalog ---"
CATALOG_ID="01900000-0000-7000-8000-000000000001"
CAT_RESP=$(curl -sS -w "\n%{http_code}" "${BASE_URL}/api/v1/catalog/rooms/${CATALOG_ID}")
CAT_BODY=$(echo "${CAT_RESP}" | head -n 1)
CAT_STATUS=$(echo "${CAT_RESP}" | tail -n 1)

if [ "${CAT_STATUS}" -eq 200 ] && echo "${CAT_BODY}" | grep -q "${CATALOG_ID}"; then
  echo "✓ Gin berhasil mengekstrak parameter :id dan mengembalikan katalog kamar (200 OK)"
else
  echo "✗ Gagal membaca katalog kamar: HTTP ${CAT_STATUS}, Body: ${CAT_BODY}"
  exit 1
fi

# 4. JSON v2 Security: Rejection of Duplicate Keys
echo "--- 4. JSON v2 Security Rejection (Duplicate Object Keys) ---"
DUP_PAYLOAD='{"room_type_id":"01900000-0000-7000-8000-000000000001","room_type_id":"01900000-0000-7000-8000-000000000002"}'
DUP_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/quotes" \
  -H "Content-Type: application/json" \
  -d "${DUP_PAYLOAD}")

if [ "${DUP_STATUS}" -eq 400 ]; then
  echo "✓ JSON v2 menolak payload kunci ganda dengan 400 Bad Request (Anti-Smuggling)"
else
  echo "✗ JSON v2 tidak menolak kunci ganda: HTTP ${DUP_STATUS}"
  exit 1
fi

# 5. Validator v10: Schema Validation & RFC 7807 Error Code
echo "--- 5. Go Validator v10 Schema Validation ---"
VAL_RESP=$(curl -sS -w "\n%{http_code}" "${BASE_URL}/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&rooms=12")
VAL_BODY=$(echo "${VAL_RESP}" | head -n 1)
VAL_STATUS=$(echo "${VAL_RESP}" | tail -n 1)

if [ "${VAL_STATUS}" -eq 400 ] && echo "${VAL_BODY}" | grep -q "INVALID_ROOM_COUNT"; then
  echo "✓ Validator v10 menolak rooms > 8 dengan 400 Bad Request dan kode RFC 7807 INVALID_ROOM_COUNT"
else
  echo "✗ Validator v10 gagal: HTTP ${VAL_STATUS}, Body: ${VAL_BODY}"
  exit 1
fi

# 6. Casbin RBAC Authorization on Gin Router
echo "--- 6. Casbin RBAC Authorization on Gin ---"
RBAC_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/admin/feature-flags" \
  -H "Authorization: Bearer ${RECEPTION_TOKEN}")

if [ "${RBAC_STATUS}" -eq 403 ]; then
  echo "✓ Casbin RBAC menolak peran receptionist pada endpoint admin (403 Forbidden)"
else
  echo "✗ Casbin RBAC tidak memblokir: HTTP ${RBAC_STATUS}"
  exit 1
fi

echo "=== [E2E SUCCESS] Seluruh pengujian modernisasi transport layer berhasil 100%! ==="
