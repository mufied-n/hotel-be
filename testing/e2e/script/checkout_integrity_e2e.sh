#!/usr/bin/env bash
# E2E: Checkout Integrity (BE-R06 quote wajib, BE-R08 idempotency atomik) + regresi alur booking.
# Wajib set BASE_URL secara eksplisit (jangan memakai port layanan lain di mesin dev).
#   BASE_URL=http://localhost:28080 ./checkout_integrity_e2e.sh
set -uo pipefail
: "${BASE_URL:?BASE_URL wajib diisi, mis. http://localhost:28080}"
RT="${ROOM_TYPE_ID:-01900000-0000-7000-8000-000000000003}"
PASS=0; FAIL=0
J=(-H "Content-Type: application/json")

check() { # name expected actual
  if [ "$2" = "$3" ]; then echo "[PASS] $1 ($3)"; PASS=$((PASS+1)); else echo "[FAIL] $1 — expected $2, got $3"; FAIL=$((FAIL+1)); fi
}
code() { curl -s -o /tmp/r.json -w "%{http_code}" "$@"; }
new_quote() { # offset nights
  local ci co; ci=$(date -d "+$1 days" +%F); co=$(date -d "+$(( $1 + ${2:-2} )) days" +%F)
  curl -s "${J[@]}" -d "{\"room_type_id\":\"$RT\",\"check_in\":\"$ci\",\"check_out\":\"$co\",\"num_rooms\":1,\"num_guests\":2}" "$BASE_URL/api/v1/quotes" |
    sed -n 's/.*"quote_id":"\([^"]*\)".*/\1/p'
}
body() { # offset quote_id [terms]
  local ci co; ci=$(date -d "+$1 days" +%F); co=$(date -d "+$(( $1 + 2 )) days" +%F)
  local q=""; [ -n "${2:-}" ] && q="\"quote_id\":\"$2\","
  echo "{${q}\"terms_accepted\":${3:-true},\"privacy_accepted\":${3:-true},\"room_type_id\":\"$RT\",\"check_in\":\"$ci\",\"check_out\":\"$co\",\"num_rooms\":1,\"num_guests\":2,\"guest_name\":\"E2E Tamu\",\"guest_email\":\"e2e@example.id\",\"guest_phone\":\"+6281234567890\"}"
}
SUFFIX=$(date +%s%N)

echo "== R06: quote wajib =="
check "R06 tanpa quote_id → 400" 400 "$(code "${J[@]}" -d "$(body 50)" "$BASE_URL/api/v1/bookings")"
grep -q QUOTE_REQUIRED /tmp/r.json && check "R06 code QUOTE_REQUIRED" ok ok || check "R06 code QUOTE_REQUIRED" ok missing
Q=$(new_quote 50)
check "R06 quote tanpa consent → 400" 400 "$(code "${J[@]}" -d "$(body 50 "$Q" false)" "$BASE_URL/api/v1/bookings")"
check "R06 quote tidak dikenal → 410" 410 "$(code "${J[@]}" -d "$(body 50 00000000-0000-7000-8000-000000000000)" "$BASE_URL/api/v1/bookings")"

echo "== R08: 20 request paralel, key sama =="
Q=$(new_quote 52); B=$(body 52 "$Q"); mkdir -p /tmp/e2e_par; rm -f /tmp/e2e_par/*
for i in $(seq 20); do
  ( curl -s -o /tmp/e2e_par/$i.json -w "%{http_code}\n" "${J[@]}" -H "Idempotency-Key: par-$SUFFIX" -d "$B" "$BASE_URL/api/v1/bookings" > /tmp/e2e_par/$i.code ) &
done; wait
IDS=$(cat /tmp/e2e_par/*.json | grep -o '"booking":{"id":"[^"]*"' | sort -u | wc -l)
OK201=$(cat /tmp/e2e_par/*.code | grep -c '^201$')
OTHER=$(cat /tmp/e2e_par/*.code | grep -vc -e '^201$' -e '^409$')
check "R08 booking unik" 1 "$IDS"
[ "$OK201" -ge 1 ] && check "R08 minimal satu 201" ok ok || check "R08 minimal satu 201" ok none
check "R08 tidak ada status selain 201/409" 0 "$OTHER"
check "R08 replay setelah selesai → 201" 201 "$(code -D /tmp/h.txt "${J[@]}" -H "Idempotency-Key: par-$SUFFIX" -d "$B" "$BASE_URL/api/v1/bookings")"
grep -qi "idempotency-replayed: true" /tmp/h.txt && check "R08 header Idempotency-Replayed" ok ok || check "R08 header Idempotency-Replayed" ok missing
check "R08 body beda key sama → 409" 409 "$(code "${J[@]}" -H "Idempotency-Key: par-$SUFFIX" -d "$(body 52 "$Q" | sed 's/E2E Tamu/Lain/')" "$BASE_URL/api/v1/bookings")"

echo "== R08: create gagal melepas key =="
code "${J[@]}" -H "Idempotency-Key: rel-$SUFFIX" -d "$(body 54 00000000-0000-7000-8000-000000000000)" "$BASE_URL/api/v1/bookings" >/dev/null
Q2=$(new_quote 54)
check "R08 retry key yang sama setelah gagal → 201" 201 "$(code "${J[@]}" -H "Idempotency-Key: rel-$SUFFIX" -d "$(body 54 "$Q2")" "$BASE_URL/api/v1/bookings")"
BID=$(sed -n 's/.*"booking":{"id":"\([^"]*\)".*/\1/p' /tmp/r.json)

echo "== Regresi: bayar → check-in → check-out =="
check "fake-pay → 200" 200 "$(code -X POST "$BASE_URL/fake-pay/$BID")"
check "check-in → 200" 200 "$(code -X POST -H 'Authorization: Bearer receptionist' "$BASE_URL/api/v1/bookings/$BID/check-in")"
check "check-out → 200" 200 "$(code -X POST -H 'Authorization: Bearer receptionist' "$BASE_URL/api/v1/bookings/$BID/check-out")"
check "guest check-in → 403" 403 "$(code -X POST "$BASE_URL/api/v1/bookings/$BID/check-in")"

echo; echo "RINGKASAN: PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
