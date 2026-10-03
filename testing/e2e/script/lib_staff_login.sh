#!/usr/bin/env bash
# Helper E2E: login staf nyata (BE-R01). Nama role literal tidak lagi diterima sebagai credential.
# Prasyarat: akun seed diberi password via `staffadmin set-password` dengan nilai $STAFF_PASSWORD yang sama.
#   STAFF_PASSWORD='min-12-karakter' BASE_URL=http://localhost:28080 ./script.sh
# Setelah `load_staff_tokens`, tersedia: T_GM_ADMIN T_RECEPTIONIST T_HOUSEKEEPING T_REVENUE_MGR T_FINANCE.

staff_login() { # username → token (stdout); gagal → exit 1
  local base="${BASE_URL:-${API_BASE_URL:?BASE_URL/API_BASE_URL wajib diisi}}"
  local tok
  tok=$(curl -s -X POST "$base/api/v1/auth/staff/login" -H "Content-Type: application/json" \
    -d "{\"username\":\"$1\",\"password\":\"${STAFF_PASSWORD:?STAFF_PASSWORD wajib diisi}\"}" |
    sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
  [ -n "$tok" ] || { echo "login staf gagal untuk $1 (cek STAFF_PASSWORD dan staffadmin set-password)" >&2; exit 1; }
  echo "$tok"
}

load_staff_tokens() {
  T_GM_ADMIN=$(staff_login admin_gm) || exit 1
  T_RECEPTIONIST=$(staff_login fo_receptionist) || exit 1
  T_HOUSEKEEPING=$(staff_login hk_lead) || exit 1
  T_REVENUE_MGR=$(staff_login rev_mgr) || exit 1
  T_FINANCE=$(staff_login fin_officer) || exit 1
}
