package catalog

// DefaultVariants mengembalikan 7 varian kamar resmi Pulang ke Uttara (95 kamar)
// yang digunakan khusus sebagai test fixture dan in-memory mock untuk unit test cepat
// tanpa memerlukan koneksi database eksternal (CI/CD sub-millisecond execution).
//
// PENTING: Pada lingkungan production (cmd/server/main.go), data varian kamar TIDAK
// diambil dari fungsi ini, melainkan langsung dibaca dan dimutasi dari/ke database
// PostgreSQL tabel `room_types` melalui catalog.NewPostgresStore(pool).
func DefaultVariants() []RoomVariant {
	return []RoomVariant{
		{
			ID:             "01900000-0000-7000-8000-000000000001",
			Code:           "sup-king",
			Name:           "Superior King Room",
			FamilyName:     "Superior",
			BedType:        "1 King Bed",
			RoomSizeSqm:    28,
			MaxCapacity:    3,
			MaxAdults:      2,
			MaxChildren:    1,
			BasePriceMinor: 550_000,
			Description:    "Kamar Superior modern dengan 1 King Bed berkualitas tinggi dan pemandangan kota Yogyakarta.",
			Amenities:      []string{"Free High-Speed Wi-Fi", "Air Conditioning", "43-inch Smart TV", "Coffee/Tea Maker", "Safe Deposit Box", "Rain Shower", "Hair Dryer"},
			Photos: []Photo{
				{URL: "https://pulangkeuttara.com/images/rooms/superior-king-1.jpg", Alt: "Superior King Bedroom"},
			},
		},
		{
			ID:             "01900000-0000-7000-8000-000000000002",
			Code:           "sup-twin",
			Name:           "Superior Twin Room",
			FamilyName:     "Superior",
			BedType:        "2 Twin Beds",
			RoomSizeSqm:    28,
			MaxCapacity:    3,
			MaxAdults:      2,
			MaxChildren:    1,
			BasePriceMinor: 550_000,
			Description:    "Kamar Superior nyaman dengan 2 Twin Beds ideal untuk perjalanan bisnis atau liburan bersama teman.",
			Amenities:      []string{"Free High-Speed Wi-Fi", "Air Conditioning", "43-inch Smart TV", "Coffee/Tea Maker", "Safe Deposit Box", "Rain Shower", "Hair Dryer"},
			Photos: []Photo{
				{URL: "https://pulangkeuttara.com/images/rooms/superior-twin-1.jpg", Alt: "Superior Twin Bedroom"},
			},
		},
		{
			ID:             "01900000-0000-7000-8000-000000000003",
			Code:           "dlx-king",
			Name:           "Deluxe King Room",
			FamilyName:     "Deluxe",
			BedType:        "1 King Bed",
			RoomSizeSqm:    34,
			MaxCapacity:    3,
			MaxAdults:      2,
			MaxChildren:    1,
			BasePriceMinor: 750_000,
			Description:    "Kamar Deluxe elegan berukuran 34 sqm dengan area duduk santai dan balkon pemandangan Gunung Merapi.",
			Amenities:      []string{"Free High-Speed Wi-Fi", "Balcony with View", "Air Conditioning", "50-inch Smart TV", "Coffee Machine", "Minibar", "Bathrobe & Slippers", "Rain Shower"},
			Photos: []Photo{
				{URL: "https://pulangkeuttara.com/images/rooms/deluxe-king-1.jpg", Alt: "Deluxe King Bedroom"},
			},
		},
		{
			ID:             "01900000-0000-7000-8000-000000000004",
			Code:           "dlx-twin",
			Name:           "Deluxe Twin Room",
			FamilyName:     "Deluxe",
			BedType:        "2 Twin Beds",
			RoomSizeSqm:    34,
			MaxCapacity:    3,
			MaxAdults:      2,
			MaxChildren:    1,
			BasePriceMinor: 750_000,
			Description:    "Kamar Deluxe lapang dengan 2 Twin Beds dan fasilitas premium untuk kenyamanan maksimal.",
			Amenities:      []string{"Free High-Speed Wi-Fi", "Balcony with View", "Air Conditioning", "50-inch Smart TV", "Coffee Machine", "Minibar", "Bathrobe & Slippers", "Rain Shower"},
			Photos: []Photo{
				{URL: "https://pulangkeuttara.com/images/rooms/deluxe-twin-1.jpg", Alt: "Deluxe Twin Bedroom"},
			},
		},
		{
			ID:             "01900000-0000-7000-8000-000000000005",
			Code:           "exc-king",
			Name:           "Executive King Room",
			FamilyName:     "Executive",
			BedType:        "1 King Bed",
			RoomSizeSqm:    42,
			MaxCapacity:    3,
			MaxAdults:      2,
			MaxChildren:    1,
			BasePriceMinor: 1_100_000,
			Description:    "Kamar Executive luas berukuran 42 sqm dengan ruang kerja ergonomis dan akses eksklusif lounge.",
			Amenities:      []string{"Free High-Speed Wi-Fi", "Executive Desk", "Espresso Machine", "Bathtub & Rain Shower", "55-inch Smart TV", "Lounge Access", "Premium Toiletries"},
			Photos: []Photo{
				{URL: "https://pulangkeuttara.com/images/rooms/executive-king-1.jpg", Alt: "Executive King Bedroom"},
			},
		},
		{
			ID:             "01900000-0000-7000-8000-000000000006",
			Code:           "jste-suite",
			Name:           "Junior Suite",
			FamilyName:     "Junior Suite",
			BedType:        "1 Super King Bed",
			RoomSizeSqm:    56,
			MaxCapacity:    4,
			MaxAdults:      2,
			MaxChildren:    2,
			BasePriceMinor: 1_650_000,
			Description:    "Suite mewah dengan ruang tamu terpisah, walk-in closet, dan kamar mandi marmer dengan bathtub.",
			Amenities:      []string{"Separate Living Area", "Walk-in Closet", "Marble Bathroom with Bathtub", "65-inch OLED TV", "Espresso Machine", "Minibar Included", "Butler Service on Demand"},
			Photos: []Photo{
				{URL: "https://pulangkeuttara.com/images/rooms/junior-suite-1.jpg", Alt: "Junior Suite Living & Bedroom"},
			},
		},
		{
			ID:             "01900000-0000-7000-8000-000000000007",
			Code:           "pste-suite",
			Name:           "Presidential Suite",
			FamilyName:     "Presidential Suite",
			BedType:        "2 King Beds",
			RoomSizeSqm:    110,
			MaxCapacity:    6,
			MaxAdults:      4,
			MaxChildren:    2,
			BasePriceMinor: 3_500_000,
			Description:    "Puncak kemewahan Pulang ke Uttara dengan 2 master bedroom, dining room, kitchenette, dan panorama 360 derajat kota Jogja.",
			Amenities:      []string{"2 Master Bedrooms", "Private Dining Room", "Kitchenette", "Jacuzzi with Skyline View", "75-inch Home Theater", "Dedicated Butler", "Private Check-in"},
			Photos: []Photo{
				{URL: "https://pulangkeuttara.com/images/rooms/presidential-suite-1.jpg", Alt: "Presidential Suite Panorama"},
			},
		},
	}
}
