#!/usr/bin/env bash
# Fixture helper untuk skrip E2E yang butuh state awal tertentu (kamar kotor,
# booking checked-in). Memakai psql + DATABASE_URL; tanpa itu skrip dilewati
# dengan pesan jelas, bukan gagal diam-diam.

db_exec() {
  if [ -z "${DATABASE_URL:-}" ] || ! command -v psql >/dev/null 2>&1; then
    echo "✗ fixture butuh DATABASE_URL (DB *_test) dan psql" >&2; return 1
  fi
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -qtAc "$1"
}

set_room_status() { # room_number status
  db_exec "UPDATE rooms SET cleanliness_status='$2' WHERE room_number='$1'"
}

# create_checked_in_booking -> echo booking_id (tamu publik + resepsionis)
create_checked_in_booking() {
  local ci co rt q res bid
  ci=$(date -u +%F); co=$(date -u -d "+2 days" +%F)
  rt="01900000-0000-7000-8000-000000000001"
  q=$(curl -s -H "Content-Type: application/json" \
    -d "{\"room_type_id\":\"$rt\",\"check_in\":\"$ci\",\"check_out\":\"$co\",\"num_rooms\":1,\"num_guests\":2}" \
    "${BASE_URL}/api/v1/quotes" | sed -n 's/.*"quote_id":"\([^"]*\)".*/\1/p')
  res=$(curl -s -H "Content-Type: application/json" \
    -d "{\"quote_id\":\"$q\",\"terms_accepted\":true,\"privacy_accepted\":true,\"room_type_id\":\"$rt\",\"check_in\":\"$ci\",\"check_out\":\"$co\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"Fixture Tamu\",\"guest_email\":\"fixture@example.id\",\"guest_phone\":\"+6281234567890\"}" \
    "${BASE_URL}/api/v1/bookings")
  bid=$(echo "$res" | sed -n 's/.*"booking":{"id":"\([^"]*\)".*/\1/p')
  curl -s -o /dev/null -X POST "${BASE_URL}/fake-pay/ref-fixture?booking_id=${bid}"
  curl -s -o /dev/null -X POST -H "Authorization: Bearer ${T_RECEPTIONIST}" "${BASE_URL}/api/v1/bookings/${bid}/check-in"
  echo "$bid"
}
