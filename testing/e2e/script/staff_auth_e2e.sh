#!/usr/bin/env bash
# E2E: Autentikasi staf (BE-R01) — credential palsu ditolak, login/me/logout nyata, regresi RBAC.
#   STAFF_PASSWORD=... BASE_URL=http://localhost:28080 ./staff_auth_e2e.sh
set -uo pipefail
: "${BASE_URL:?BASE_URL wajib diisi, mis. http://localhost:28080}"
source "$(dirname "${BASH_SOURCE[0]}")/lib_staff_login.sh"
PASS=0; FAIL=0
check() { if [ "$2" = "$3" ]; then echo "[PASS] $1 ($3)"; PASS=$((PASS+1)); else echo "[FAIL] $1 — expected $2, got $3"; FAIL=$((FAIL+1)); fi; }
code() { curl -s -o /tmp/sa.json -w "%{http_code}" "$@"; }
J=(-H "Content-Type: application/json")

echo "== Credential palsu =="
for hdr in "Authorization: Bearer gm_admin" "Authorization: Bearer finance" "Authorization: Bearer staff:gm_admin:x" \
           "X-User-Role: gm_admin" "X-User-Role: finance" "X-Testing-Role: true"; do
  check "palsu [$hdr] → 403" 403 "$(code -H "$hdr" "$BASE_URL/api/v1/housekeeping/rooms")"
done
check "palsu X-User-Role+X-Internal-Secret → 403" 403 "$(code -H 'X-User-Role: gm_admin' -H 'X-Internal-Secret: internal-service-secret' "$BASE_URL/api/v1/finance/reconciliations")"
check "token stf_ palsu → 401" 401 "$(code -H 'Authorization: Bearer stf_forgedforgedforgedforged' "$BASE_URL/api/v1/housekeeping/rooms")"

echo "== Login =="
check "login password salah → 401" 401 "$(code "${J[@]}" -d '{"username":"fo_receptionist","password":"salah-salah-salah"}' "$BASE_URL/api/v1/auth/staff/login")"
check "login user tak dikenal → 401" 401 "$(code "${J[@]}" -d '{"username":"ghost","password":"salah-salah-salah"}' "$BASE_URL/api/v1/auth/staff/login")"
check "login body kosong → 400" 400 "$(code "${J[@]}" -d '{}' "$BASE_URL/api/v1/auth/staff/login")"
check "login sukses → 200" 200 "$(code -D /tmp/sa.h "${J[@]}" -d "{\"username\":\"fo_receptionist\",\"password\":\"$STAFF_PASSWORD\"}" "$BASE_URL/api/v1/auth/staff/login")"
grep -qi "cache-control: no-store" /tmp/sa.h && check "login no-store" ok ok || check "login no-store" ok missing
TOK=$(sed -n 's/.*"token":"\([^"]*\)".*/\1/p' /tmp/sa.json)

echo "== Sesi nyata =="
check "me dengan token → 200" 200 "$(code -H "Authorization: Bearer $TOK" "$BASE_URL/api/v1/auth/staff/me")"
grep -q '"role":"receptionist"' /tmp/sa.json && check "me role dari server" ok ok || check "me role dari server" ok missing
check "me tanpa token → 401" 401 "$(code "$BASE_URL/api/v1/auth/staff/me")"
check "receptionist akses finance (dilarang) → 403" 403 "$(code -H "Authorization: Bearer $TOK" "$BASE_URL/api/v1/finance/reconciliations")"
check "gm_admin akses papan housekeeping → 200" 200 "$(code -H "Authorization: Bearer ${T_GM_ADMIN:-$(staff_login admin_gm)}" "$BASE_URL/api/v1/housekeeping/rooms")"

echo "== Logout =="
check "logout → 204" 204 "$(code -X POST -H "Authorization: Bearer $TOK" "$BASE_URL/api/v1/auth/staff/logout")"
check "token setelah logout → 401" 401 "$(code -H "Authorization: Bearer $TOK" "$BASE_URL/api/v1/auth/staff/me")"

echo; echo "RINGKASAN: PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
