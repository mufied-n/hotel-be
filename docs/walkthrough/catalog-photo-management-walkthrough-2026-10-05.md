# Walkthrough: Catalog Photo Management & Staff Catalog Editor

- **Tanggal:** 2026-10-05
- **Dokumen terkait:** [`docs/tech/catalog-photo-management-architecture-2026-10-05.md`](../tech/catalog-photo-management-architecture-2026-10-05.md)

## Milestone
| # | Milestone | Status |
|---|-----------|--------|
| 1 | Audit: BE sudah punya CRUD katalog + `ff_catalog_write`; FE BFF read-only; editor FE hanya simulasi | Selesai |
| 2 | BE (TDD): `ValidateVariant` + batas/aturan foto, dipakai Memory & Postgres store | Selesai |
| 3 | FE BFF: allowlist `POST/PUT/DELETE catalog/rooms` | Selesai |
| 4 | FE BFF: `POST /api/bff/staff/catalog/photos` (peran, magic bytes, R2) | Selesai |
| 5 | FE: kompresi di browser + editor (upload, hapus, urut, simpan, varian baru, hapus) | Selesai |
| 6 | Unit + E2E + lint + typecheck | Selesai |
| 7 | Deploy + set env R2 di VPS + rotate kredensial | **Menunggu** |

## Perubahan per file

### Backend `current-booking`
| File | Perubahan |
|------|-----------|
| `internal/catalog/catalog.go` | Tambah `MaxPhotosPerVariant`, `MaxPhotoAltLen`, `ValidateVariant`; MemoryStore memakainya. |
| `internal/catalog/postgres.go` | Create/Update memakai `ValidateVariant`; hapus import `strings` yang tak terpakai. |
| `internal/catalog/validate_test.go` | Baru: table-driven 14 kasus + penolakan di MemoryStore. |

### Frontend `hotel-fe`
| File | Perubahan |
|------|-----------|
| `server/utils/staff-capabilities.ts` | Allowlist catalog POST/PUT/DELETE. |
| `server/utils/photo-upload.ts` | Baru: sniff magic bytes, validasi slug, nama key R2. |
| `server/api/bff/staff/catalog/photos.post.ts` | Baru: endpoint upload khusus `revenue_mgr`/`gm_admin`. |
| `app/utils/image-compress.ts` | Baru: resize 1600px + JPEG bertahap ≤ 900 KB di browser. |
| `app/pages/staff/catalog.vue` | Ditulis ulang: simpan ke backend, upload/hapus/urut foto, varian baru, hapus. |
| `tests/unit/photo-upload.test.ts`, `tests/unit/staff-routes.test.ts` | Unit test baru/diperluas. |
| `tests/e2e/booking.spec.ts` | E2E editor katalog (mode sample). |

## Catatan teknis dan temuan
- `structuredClone(toRaw(...))` gagal pada state reaktif bersarang (foto) → diganti `JSON.parse(JSON.stringify())`; tombol simpan sempat tidak berefek.
- Data sample katalog punya `base_price_minor = 0`, sehingga validasi HTML5 (`min=1`) memblokir submit di mode sample sampai harga diisi. E2E mengisi harga; bukan bug baru, tetapi data sample bisa diperbaiki terpisah.
- `sed -i` gagal pada mount repo (izin); edit dilakukan lewat tool/python.

## Perintah verifikasi
```bash
# Backend
go vet ./internal/catalog/ && go test -cover ./internal/catalog/ ./internal/api/...
# Frontend
npm run lint && npm run typecheck && npm run test:unit && npm run test:e2e
```

## Checklist go-live (manual)
- [ ] Merge/deploy BE dan FE.
- [ ] `/opt/hotel-fe/.env`: `NUXT_PUBLIC_OPERATIONS_MODE=api`, `NUXT_R2_*` (token khusus bucket `pulang`, Object Read & Write).
- [ ] Rotate kredensial R2 yang pernah tertulis di chat.
- [ ] nginx: `client_max_body_size 5m;` untuk `/api/bff/staff/catalog/photos`.
- [ ] Uji sebagai `revenue_mgr`: upload → simpan → cek card di `/booking/results`.
- [ ] Uji sebagai `receptionist`: upload/simpan harus 403.
