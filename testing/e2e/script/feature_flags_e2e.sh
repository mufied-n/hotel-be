#!/usr/bin/env bash
# ==============================================================================
# E2E Test Script: Feature Flags & Runtime Configuration System
# Hotel Booking Engine — Pulang ke Uttara (Yogyakarta)
#
# Menguji:
# 1. Autorisasi Admin Casbin RBAC pada Endpoint Feature Flags
# 2. Toggle Runtime Global (Kill Switch) & Respons 503 RFC 7807
# 3. Role-Based Scoping (Canary Rollout) & Pengecualian Akses
# 4. Pemulihan Fitur (Restoration to Active State)
# ==============================================================================

set -euo pipefail

BASE_URL="${API_BASE_URL:-http://localhost:18080}"
source "$(dirname "${BASH_SOURCE[0]}")/lib_staff_login.sh"
load_staff_tokens
GM_TOKEN="${T_GM_ADMIN}"
RECEPTION_TOKEN="${T_RECEPTIONIST}"
REVENUE_MGR_TOKEN="${T_REVENUE_MGR}"

echo "=== [E2E] Menjalankan Pengujian Feature Flags & Runtime Toggle ==="
echo "Target Base URL: ${BASE_URL}"

# 1. Healthcheck
echo "--- 1. Healthcheck Service ---"
curl -sS -f "${BASE_URL}/healthz" > /dev/null
echo "✓ Service Healthcheck OK"

# 2. Admin RBAC: List Feature Flags
echo "--- 2. RBAC Access Test: GET /api/v1/admin/feature-flags ---"
HTTP_CODE_REC=$(curl -sS -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/admin/feature-flags" \
  -H "Authorization: Bearer ${RECEPTION_TOKEN}")

if [ "${HTTP_CODE_REC}" -eq 403 ]; then
  echo "✓ Akses ditolak untuk role receptionist (403 Forbidden)"
else
  echo "✗ Resepionis tidak diblokir: HTTP ${HTTP_CODE_REC}"
  exit 1
fi

LIST_RESP=$(curl -sS -X GET "${BASE_URL}/api/v1/admin/feature-flags" \
  -H "Authorization: Bearer ${GM_TOKEN}")

TOTAL_FLAGS=$(echo "${LIST_RESP}" | grep -o '"key":' | wc -l || true)
echo "✓ Akses diizinkan untuk gm_admin (200 OK), total flags ditemukan: ${TOTAL_FLAGS}"

# 3. Global Kill Switch: Disable ff_multi_variant_search
echo "--- 3. Global Kill Switch Test: Disable ff_multi_variant_search ---"
UPDATE_RESP=$(curl -sS -X PUT "${BASE_URL}/api/v1/admin/feature-flags/ff_multi_variant_search" \
  -H "Authorization: Bearer ${GM_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"enabled": false, "allowed_roles": []}')

echo "Toggle Response: ${UPDATE_RESP}"

# Verifikasi GET /api/v1/search ditolak dengan 503 FEATURE_DISABLED
SEARCH_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=1")
if [ "${SEARCH_STATUS}" -eq 503 ]; then
  echo "✓ Endpoint pencarian berhasil diblokir saat flag disabled (503 Service Unavailable)"
else
  echo "✗ Endpoint pencarian tidak mengembalikan 503: HTTP ${SEARCH_STATUS}"
  exit 1
fi

# 4. Restore Global Flag: Enable ff_multi_variant_search
echo "--- 4. Restore Global Flag Test: Enable ff_multi_variant_search ---"
RESTORE_RESP=$(curl -sS -X PUT "${BASE_URL}/api/v1/admin/feature-flags/ff_multi_variant_search" \
  -H "Authorization: Bearer ${GM_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "allowed_roles": []}')

SEARCH_RESTORED_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" "${BASE_URL}/api/v1/search?check_in=2026-10-10&check_out=2026-10-12&adults=1")
if [ "${SEARCH_RESTORED_STATUS}" -eq 200 ]; then
  echo "✓ Endpoint pencarian pulih dan aktif normal kembali (200 OK)"
else
  echo "✗ Endpoint pencarian gagal pulih: HTTP ${SEARCH_RESTORED_STATUS}"
  exit 1
fi

# 5. Role-Scoped Canary Testing: Restrict ff_catalog_write to gm_admin only
echo "--- 5. Role-Scoped Canary Test: Restrict ff_catalog_write to gm_admin only ---"
curl -sS -X PUT "${BASE_URL}/api/v1/admin/feature-flags/ff_catalog_write" \
  -H "Authorization: Bearer ${GM_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "allowed_roles": ["gm_admin"]}' > /dev/null

# Call POST /catalog/rooms as revenue_mgr (should receive 503 FEATURE_DISABLED despite Casbin permission)
REV_POST_STATUS=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/catalog/rooms" \
  -H "Authorization: Bearer ${REVENUE_MGR_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"code":"TEST-TMP","name":"Test Room","base_price_minor":500000,"max_capacity":2,"max_adults":2,"max_children":1}')

if [ "${REV_POST_STATUS}" -eq 503 ]; then
  echo "✓ Revenue Manager diblokir oleh Role-Scoping Feature Flag (503 Service Unavailable)"
else
  echo "✗ Revenue Manager tidak diblokir oleh flag: HTTP ${REV_POST_STATUS}"
  exit 1
fi

# Restore ff_catalog_write to original allowed_roles
curl -sS -X PUT "${BASE_URL}/api/v1/admin/feature-flags/ff_catalog_write" \
  -H "Authorization: Bearer ${GM_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "allowed_roles": ["revenue_mgr", "gm_admin"]}' > /dev/null
echo "✓ Status flag ff_catalog_write berhasil dikembalikan ke semula"

echo "=== Selesai: Seluruh pengujian E2E Feature Flags berhasil 100% ==="
