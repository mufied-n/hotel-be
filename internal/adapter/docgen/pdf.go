package docgen

import (
	"fmt"
	"math"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// VoucherData memuat informasi lengkap reservasi untuk dirender pada Confirmation Voucher PDF.
type VoucherData struct {
	Reference     string
	BookingID     string
	GuestName     string
	GuestEmail    string
	GuestPhone    string
	RoomTypeName  string
	RatePlanName  string
	Inclusions    string
	CheckIn       time.Time
	CheckOut      time.Time
	Nights        int
	NumRooms      int
	Adults        int
	Children      int
	TotalPaidIDR  int64
	PaymentMethod string
	PaidAt        time.Time
	QRToken       string
}

// InvoiceData memuat rincian faktur pajak resmi PBJT Kabupaten Sleman.
type InvoiceData struct {
	InvoiceNumber string
	Reference     string
	BookingID     string
	GuestName     string
	GuestEmail    string
	RoomTypeName  string
	CheckIn       time.Time
	CheckOut      time.Time
	Nights        int
	NumRooms      int
	NetRoomChargesIDR int64 // Dasar Pengenaan Pajak (DPP)
	ServiceChargeIDR  int64 // 10% dari DPP
	PBJTTaxIDR        int64 // 10% Pajak Barang & Jasa Tertentu (Kabupaten Sleman)
	TotalPaidIDR      int64 // Total Bruto Pelunasan
	PaymentMethod     string
	PaidAt            time.Time
}

// CalculatePBJTTaxes menghitung rincian DPP, Service Charge 10%, dan PBJT 10% Sleman tanpa pembulatan desimal.
func CalculatePBJTTaxes(totalPaidIDR int64) (dpp, service, pbjt int64) {
	if totalPaidIDR <= 0 {
		return 0, 0, 0
	}
	// Total = DPP * 1.10 (Service) * 1.10 (PBJT) = DPP * 1.21
	dppFloat := float64(totalPaidIDR) / 1.21
	dpp = int64(math.Round(dppFloat))
	service = int64(math.Round(float64(dpp) * 0.10))
	pbjt = totalPaidIDR - dpp - service
	return dpp, service, pbjt
}

// GenerateVoucherPDF merender berkas Confirmation Voucher resmi standar A4 secara in-memory.
func GenerateVoucherPDF(data VoucherData, qrPNG []byte) ([]byte, error) {
	cfg := config.NewBuilder().
		WithPageNumber().
		WithLeftMargin(15).
		WithRightMargin(15).
		WithTopMargin(15).
		WithBottomMargin(15).
		Build()

	m := maroto.New(cfg)

	// 1. Header Properti Hotel
	m.AddRows(
		row.New(24).Add(
			col.New(8).Add(
				text.New("PULANG KE UTTARA", props.Text{Style: fontstyle.Bold, Size: 18, Color: &props.Color{Red: 33, Green: 50, Blue: 40}}),
				text.New("Boutique Hotel Yogyakarta — 4 Stars", props.Text{Size: 9, Style: fontstyle.Italic, Color: &props.Color{Red: 100, Green: 100, Blue: 100}}),
				text.New("Jl. Kaliurang Km 5, Sleman, D.I. Yogyakarta 55281 | Telp: +62 274 5020888", props.Text{Size: 8}),
				text.New("Email: stay@pulangkeuttara.id | Web: https://pulangkeuttara.id", props.Text{Size: 8}),
			),
			col.New(4).Add(
				text.New("BOOKING VOUCHER", props.Text{Style: fontstyle.Bold, Size: 12, Align: align.Right, Color: &props.Color{Red: 180, Green: 120, Blue: 40}}),
				text.New(fmt.Sprintf("Ref: %s", data.Reference), props.Text{Style: fontstyle.Bold, Size: 11, Align: align.Right}),
				text.New("Status: CONFIRMED & PAID", props.Text{Style: fontstyle.Bold, Size: 9, Align: align.Right, Color: &props.Color{Red: 34, Green: 139, Blue: 34}}),
				text.New(fmt.Sprintf("Issued: %s", time.Now().Format("02 Jan 2006 15:04 WIB")), props.Text{Size: 8, Align: align.Right}),
			),
		),
		row.New(4), // Spacing
	)

	// 2. Baris Detail Tamu & Inap
	m.AddRows(
		row.New(16).Add(
			col.New(6).Add(
				text.New("GUEST INFORMATION", props.Text{Style: fontstyle.Bold, Size: 9, Color: &props.Color{Red: 80, Green: 80, Blue: 80}}),
				text.New(fmt.Sprintf("Primary Guest: %s", data.GuestName), props.Text{Style: fontstyle.Bold, Size: 9}),
				text.New(fmt.Sprintf("Contact: %s (%s)", data.GuestPhone, data.GuestEmail), props.Text{Size: 8}),
				text.New(fmt.Sprintf("Occupancy: %d Adult(s), %d Child(ren)", data.Adults, data.Children), props.Text{Size: 8}),
			),
			col.New(6).Add(
				text.New("STAY SCHEDULE", props.Text{Style: fontstyle.Bold, Size: 9, Color: &props.Color{Red: 80, Green: 80, Blue: 80}}),
				text.New(fmt.Sprintf("Check-in:  %s (from 14:00 WIB)", data.CheckIn.Format("02 Jan 2006")), props.Text{Style: fontstyle.Bold, Size: 9}),
				text.New(fmt.Sprintf("Check-out: %s (until 12:00 WIB)", data.CheckOut.Format("02 Jan 2006")), props.Text{Style: fontstyle.Bold, Size: 9}),
				text.New(fmt.Sprintf("Duration: %d Night(s), %d Room(s)", data.Nights, data.NumRooms), props.Text{Size: 8}),
			),
		),
		row.New(4),
	)

	// 3. Detail Kamar & Paket
	inclusions := data.Inclusions
	if inclusions == "" {
		inclusions = "Standard Room Inclusions & WiFi"
	}
	m.AddRows(
		row.New(16).Add(
			col.New(8).Add(
				text.New("RESERVATION DETAILS", props.Text{Style: fontstyle.Bold, Size: 9, Color: &props.Color{Red: 80, Green: 80, Blue: 80}}),
				text.New(fmt.Sprintf("Room Type: %s", data.RoomTypeName), props.Text{Style: fontstyle.Bold, Size: 10}),
				text.New(fmt.Sprintf("Package: %s", data.RatePlanName), props.Text{Size: 9}),
				text.New(fmt.Sprintf("Inclusions: %s", inclusions), props.Text{Size: 8}),
			),
			col.New(4).Add(
				text.New("TOTAL PAID", props.Text{Style: fontstyle.Bold, Size: 9, Color: &props.Color{Red: 80, Green: 80, Blue: 80}, Align: align.Right}),
				text.New(fmt.Sprintf("IDR %s", formatRupiah(data.TotalPaidIDR)), props.Text{Style: fontstyle.Bold, Size: 12, Align: align.Right, Color: &props.Color{Red: 33, Green: 50, Blue: 40}}),
				text.New(fmt.Sprintf("Via: %s", data.PaymentMethod), props.Text{Size: 8, Align: align.Right}),
			),
		),
		row.New(6),
	)

	// 4. QR Code Verifikasi Meja Depan
	if len(qrPNG) > 0 {
		m.AddRows(
			row.New(32).Add(
				col.New(3).Add(
					image.NewFromBytes(qrPNG, extension.Png, props.Rect{
						Center:  true,
						Percent: 90,
					}),
				),
				col.New(9).Add(
					text.New("FAST FRONT DESK CHECK-IN QR", props.Text{Style: fontstyle.Bold, Size: 9}),
					text.New("Tunjukkan QR Code ini kepada resepsionis saat kedatangan untuk proses Express Check-in instan.", props.Text{Size: 8}),
					text.New(fmt.Sprintf("Security Digital Signature: %s...", data.QRToken[:min(len(data.QRToken), 32)]), props.Text{Size: 7, Style: fontstyle.Italic, Color: &props.Color{Red: 120, Green: 120, Blue: 120}}),
					text.New("Dokumen ini sah dan diterbitkan secara digital oleh Hotel Pulang ke Uttara.", props.Text{Size: 7}),
				),
			),
			row.New(4),
		)
	}

	// 5. Syarat & Kebijakan Menginap
	m.AddRows(
		row.New(24).Add(
			col.New(12).Add(
				text.New("HOTEL POLICIES & INFORMATION", props.Text{Style: fontstyle.Bold, Size: 9, Color: &props.Color{Red: 80, Green: 80, Blue: 80}}),
				text.New("1. Waktu Check-in resmi mulai pukul 14:00 WIB. Waktu Check-out maksimal pukul 12:00 WIB.", props.Text{Size: 7}),
				text.New("2. Sarapan pagi disajikan pukul 06:00 - 10:00 WIB di Restoran Lantai 1.", props.Text{Size: 7}),
				text.New("3. Seluruh kamar merupakan 100% Non-Smoking. Merokok di dalam kamar dikenakan biaya pembersihan IDR 1.500.000.", props.Text{Size: 7}),
				text.New("4. Mohon siapkan kartu identitas asli (KTP / Paspor) yang masih berlaku saat proses registrasi check-in.", props.Text{Size: 7}),
			),
		),
	)

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("docgen: generate voucher pdf: %w", err)
	}

	return doc.GetBytes(), nil
}

// GenerateInvoicePDF merender berkas Faktur Pajak Daerah Resmi (PBJT) Kabupaten Sleman standar A4.
func GenerateInvoicePDF(data InvoiceData) ([]byte, error) {
	cfg := config.NewBuilder().
		WithPageNumber().
		WithLeftMargin(15).
		WithRightMargin(15).
		WithTopMargin(15).
		WithBottomMargin(15).
		Build()

	m := maroto.New(cfg)

	// 1. Header Faktur & Legalitas Wajib Pajak
	m.AddRows(
		row.New(26).Add(
			col.New(7).Add(
				text.New("PT PULANG UTTARA SEJAHTERA", props.Text{Style: fontstyle.Bold, Size: 14, Color: &props.Color{Red: 33, Green: 50, Blue: 40}}),
				text.New("Unit Usaha: Hotel Pulang ke Uttara (Bintang 4)", props.Text{Size: 9, Style: fontstyle.Bold}),
				text.New("NPWPD : 01.234.567.8-542.000 (Kabupaten Sleman, D.I.Y)", props.Text{Size: 8, Style: fontstyle.Bold}),
				text.New("Alamat: Jl. Kaliurang Km 5, Sleman, D.I. Yogyakarta 55281", props.Text{Size: 8}),
			),
			col.New(5).Add(
				text.New("FAKTUR PAJAK DAERAH (PBJT)", props.Text{Style: fontstyle.Bold, Size: 11, Align: align.Right, Color: &props.Color{Red: 180, Green: 120, Blue: 40}}),
				text.New(fmt.Sprintf("No: %s", data.InvoiceNumber), props.Text{Style: fontstyle.Bold, Size: 10, Align: align.Right}),
				text.New(fmt.Sprintf("Booking Ref: %s", data.Reference), props.Text{Size: 9, Align: align.Right}),
				text.New(fmt.Sprintf("Tanggal: %s", data.PaidAt.Format("02 Jan 2006 15:04 WIB")), props.Text{Size: 8, Align: align.Right}),
			),
		),
		row.New(4),
	)

	// 2. Data Pelanggan / Tamu
	m.AddRows(
		row.New(14).Add(
			col.New(6).Add(
				text.New("DITERBITKAN KEPADA:", props.Text{Style: fontstyle.Bold, Size: 8, Color: &props.Color{Red: 100, Green: 100, Blue: 100}}),
				text.New(fmt.Sprintf("Nama Tamu : %s", data.GuestName), props.Text{Style: fontstyle.Bold, Size: 9}),
				text.New(fmt.Sprintf("Email     : %s", data.GuestEmail), props.Text{Size: 8}),
			),
			col.New(6).Add(
				text.New("RINCIAN MENGINAP:", props.Text{Style: fontstyle.Bold, Size: 8, Color: &props.Color{Red: 100, Green: 100, Blue: 100}}),
				text.New(fmt.Sprintf("Kamar     : %s (%d Kamar)", data.RoomTypeName, data.NumRooms), props.Text{Size: 8}),
				text.New(fmt.Sprintf("Periode   : %s s/d %s (%d Malam)", data.CheckIn.Format("02/01/2006"), data.CheckOut.Format("02/01/2006"), data.Nights), props.Text{Size: 8}),
			),
		),
		row.New(6),
	)

	// 3. Tabel Rincian Keuangan PBJT (Sleman Perda No. 7/2023)
	m.AddRows(
		row.New(8).Add(
			col.New(8).Add(text.New("DESKRIPSI TRANSAKSI", props.Text{Style: fontstyle.Bold, Size: 8})),
			col.New(4).Add(text.New("JUMLAH (IDR)", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right})),
		),
		row.New(8).Add(
			col.New(8).Add(text.New("Biaya Bersih Kamar Hotel (Dasar Pengenaan Pajak / DPP)", props.Text{Size: 8})),
			col.New(4).Add(text.New(formatRupiah(data.NetRoomChargesIDR), props.Text{Size: 8, Align: align.Right})),
		),
		row.New(8).Add(
			col.New(8).Add(text.New("Biaya Layanan Hotel (Service Charge 10% x DPP)", props.Text{Size: 8})),
			col.New(4).Add(text.New(formatRupiah(data.ServiceChargeIDR), props.Text{Size: 8, Align: align.Right})),
		),
		row.New(8).Add(
			col.New(8).Add(text.New("Pajak Barang & Jasa Tertentu - Jasa Perhotelan (PBJT Sleman 10%)", props.Text{Size: 8})),
			col.New(4).Add(text.New(formatRupiah(data.PBJTTaxIDR), props.Text{Size: 8, Align: align.Right})),
		),
		row.New(2),
		row.New(10).Add(
			col.New(8).Add(text.New("TOTAL PELUNASAN (NETTO)", props.Text{Style: fontstyle.Bold, Size: 9, Color: &props.Color{Red: 33, Green: 50, Blue: 40}})),
			col.New(4).Add(text.New(fmt.Sprintf("IDR %s", formatRupiah(data.TotalPaidIDR)), props.Text{Style: fontstyle.Bold, Size: 10, Align: align.Right, Color: &props.Color{Red: 33, Green: 50, Blue: 40}})),
		),
		row.New(6),
	)

	// 4. Status Pelunasan & Stempel Digital
	m.AddRows(
		row.New(16).Add(
			col.New(6).Add(
				text.New(fmt.Sprintf("Status Pelunasan : LUNAS (PAID via %s)", data.PaymentMethod), props.Text{Style: fontstyle.Bold, Size: 8, Color: &props.Color{Red: 34, Green: 139, Blue: 34}}),
				text.New(fmt.Sprintf("Waktu Pelunasan  : %s", data.PaidAt.Format("02 Jan 2006 15:04:05 WIB")), props.Text{Size: 8}),
				text.New("Faktur ini merupakan bukti pemungutan pajak daerah yang sah sesuai peraturan perundangan yang berlaku.", props.Text{Size: 7, Style: fontstyle.Italic}),
			),
			col.New(6).Add(
				text.New("DITERBITKAN SECARA ELEKTRONIK", props.Text{Style: fontstyle.Bold, Size: 8, Align: align.Right}),
				text.New("BAGIAN KEUANGAN & PAJAK", props.Text{Size: 8, Align: align.Right}),
				text.New("HOTEL PULANG KE UTTARA SLEMAN", props.Text{Size: 7, Align: align.Right, Style: fontstyle.Bold}),
			),
		),
	)

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("docgen: generate invoice pdf: %w", err)
	}

	return doc.GetBytes(), nil
}

// formatRupiah memformat bilangan bulat ke notasi pemisah ribuan titik (misal: 1.250.000).
func formatRupiah(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var res []byte
	l := len(s)
	for i := 0; i < l; i++ {
		if i > 0 && (l-i)%3 == 0 {
			res = append(res, '.')
		}
		res = append(res, s[i])
	}
	if neg {
		return "-" + string(res)
	}
	return string(res)
}
