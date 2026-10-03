# PRD — Identitas staff, izin dan audit perubahan

Dokumen teknis: [TECH-F12](../tech/12-staff-identity-permissions-and-audit-architecture-2026-10-03.md).

ID: PRD-F12-2026-10-03. Status: PROPOSED / REQUIRES OWNER REVIEW. Tanggal: 3 Oktober 2026 (Asia/Jakarta).
Owner: BE identity/security + GM; existing Casbin owner. Prioritas: P1 sebelum live sesuai scope; pilot sebagai release gate.
Pasangan: [SRS-F12](../srs/12-staff-identity-permissions-and-audit-2026-10-03.md). Registry: [PRD index](README.md).

## Konteks, sumber dan batas bukti

Existing RBAC doc dan G14 audit: header identity belum trusted/fail-open. Ini persyaratan lanjutan, bukan klaim source saat ini belum diperbaiki.

Rujukan: [riset parity FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/research/booking-flow-parity.md), [audit gap](../gap/README.md), [QA FE](/mnt/code/projects/jobs/pulang/mimiking-booking-secure/docs/qa/frontend-ui-report.md). Gap terkait: **BE-G14, G15, G20**. Riset vendor berhenti sebelum submit/pembayaran; tidak mengklaim login/dashboard/receipt/refund vendor telah diamati. Hasil audit adalah snapshot, bukan re-audit backend terbaru. Dokumen baru tidak mengubah status approved dokumen sebelumnya.

## Masalah dan outcome

Permission roles tidak cukup jika identity dapat dipalsukan atau protected route berjalan ketika enforcer gagal.

Outcome: Trusted auth/session, fail-closed Casbin, property scope, role assignment, actor audit, protected ops boundaries. Keberhasilan dinilai dari acceptance dan bukti sistem terkait; tersedia UI atau PRD approved tidak berarti production-ready.

## Persona dan authority

receptionist/housekeeping/revenue_mgr/finance/gm_admin dari matriks approved; subject provider tidak sama dengan staff. Permission akhir mengikuti owner RBAC, dan semua read/mutation tetap diperiksa server. Tamu utama yang memesan, penghuni kamar, dan staff adalah identitas berbeda; occupancy bukan bukti ownership akun.

## User journey

Verified staff login → derive role server → authorize resource → mutate audited → revoke when inactive.

Entry, context dan recovery harus dapat dinavigasi dari webapp booking. Loading, empty, validation, forbidden, offline dan concurrent-change memiliki pesan/aksi yang sesuai, tanpa mengklaim hasil transaksi yang belum diketahui.

## Scope dan batas

Termasuk: Trusted auth/session, fail-closed Casbin, property scope, role assignment, actor audit, protected ops boundaries.

Di luar scope: Tidak mengganti matriks Casbin approved tanpa amendment; guest endpoint dipisah dari staff.

Prototype memakai fixture dengan label demo/no-reservation. Live membutuhkan adapter backend, data owner dan evidence gate terkait; mock/demo status tidak menjadi authority operasional.

## Kebutuhan produk dan acceptance

### F12-AC-01

Kebutuhan: Identity diverifikasi server dari selected IdP/session; X-User-ID/X-User-Role dan literal Bearer role publik tidak memberi authority.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F12-FR-01.

### F12-AC-02

Kebutuhan: Enforcer init/load failure fail-closed; readiness tidak hijau untuk protected ops ketika auth dependency unusable.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F12-FR-02.

### F12-AC-03

Kebutuhan: Subject active/role/property scope dicek; inactive/revoked staff ditolak walau token valid structurally.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F12-FR-03.

### F12-AC-04

Kebutuhan: Authorization deny by default per action/resource; finance refund, revenue tariff, reception stay dan GM exception punya permission spesifik.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F12-FR-04.

### F12-AC-05

Kebutuhan: Role changes tidak self-escalate; approval/step-up untuk aksi sensitif ditetapkan owner. Audit gagal ditulis membuat sensitive mutation gagal atomically.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F12-FR-05.

### F12-AC-06

Kebutuhan: Audit menyimpan actor verified, action, resource, before/after redacted, reason/time/request_id/version; immutable kepada caller biasa.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F12-FR-06.

### F12-AC-07

Kebutuhan: Staff logout/expiry/revoke dan provider event credentials terpisah; secrets/token audit/log di-redact dan semua private response no-store.

Acceptance: skenario normal membuktikan kebutuhan tersebut pada sumber otoritatif; input/akses tidak sah atau state berubah ditolak tanpa efek parsial. QA mencatat trigger, expected result dan actual evidence. Trace: SRS-F12-FR-07.

## Skenario review wajib

- F12-SC-1: Forged headers/Bearer gm_admin ditolak.
- F12-SC-2: Enforcer nil/init failure protected route closed.
- F12-SC-3: Role matrix + property/guest cross-resource deny.
- F12-SC-4: Inactive staff dan audit write failure tidak mutate.

Skenario negatif/concurrency harus diperiksa dengan data dua subject berbeda dan replay ketika relevan. Hasil fixture dipisah dari DB/provider sandbox, rendered browser dan actual physical devices.

## NFR dan indikator keberhasilan

- UX: keyboard/labels/focus/error recovery, status announcement yang tidak spam, reduced-motion dan viewport 360/390/768/1440 sesuai halaman; physical-device hasil dicatat terpisah.
- Integritas: tidak ada duplicate stock/charge/refund/delivery logis untuk replay yang sama; semua counter/persisted snapshots konsisten.
- Privacy: PII minimum, response privat no-store, role/ownership enforced; retention ditetapkan owner.
- Operasional: timeout/retry guidance, structured event tanpa secret, rollback/recovery reason dapat ditelusuri.
- Ukur: completion/failure/recovery per skenario, resource/money discrepancy, unauthorized leakage (harus nol pada suite), retry/backlog jika async. Target latency/SLA numerik ditetapkan setelah baseline dan persetujuan owner; tidak diasumsikan dari screenshot.

## Dependensi, handoff dan rollout

Tidak ada fitur prerequisite; keputusan identity/hotel dapat membatasi integrasi.

FE: typed view model dan scenario matrix; BE: DTO/domain/transactions/auth; QA: test cross-owner, failure/replay dan data effects; hotel: keputusan policy/data; ops: provider/deployment/recovery bila relevan.
Implementasikan fixture dahulu untuk review bila aman; sebelum live, requirement domain harus diverifikasi nyata. Tidak mengaktifkan route staff/provider dev melalui UI publik.

## Keputusan terbuka dan approval

Staff IdP/login method, MFA/step-up, role provisioning approvals, audit retention/export dan property scope.

Lihat [register kontrak dan konflik](../srs/00-shared-contracts-and-decisions-2026-10-03.md). Owner mengisi pilihan, alasan, tanggal/effective scope dan approver. Dokumen ini tidak menganggap TTL/provider/tarif/cancellation/refund/privacy sudah disetujui dari percakapan.

## Definition of ready / done

Ready: owner/source authoritative, keputusan terbuka yang memengaruhi kontrak selesai, SRS/OpenAPI review, contoh dan skenario disepakati.
Done: source changes + owner acceptance + verification evidence; hasil NOT RUN/FAILED masih terbuka. Untuk Go, ikuti AGENTS lifecycle/TDD coverage >=80% pada package baru/refinement, vet, race dan real-DB/E2E sesuai risiko.
Status implementasi setiap fitur diisi lewat walkthrough saat pekerjaan diotorisasi; paket spesifikasi ini tidak melakukan implementasi fitur.

