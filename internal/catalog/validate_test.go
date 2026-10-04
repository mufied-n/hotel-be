package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func validVariant() RoomVariant {
	return RoomVariant{Code: "x-king", Name: "X King", MaxCapacity: 2, BasePriceMinor: 100000}
}

func TestValidateVariant(t *testing.T) {
	manyPhotos := make([]Photo, MaxPhotosPerVariant+1)
	for i := range manyPhotos {
		manyPhotos[i] = Photo{URL: fmt.Sprintf("/asset/rooms/bay/%d.jpg", i), Alt: "a"}
	}

	tests := []struct {
		name   string
		mutate func(v *RoomVariant)
		want   error
	}{
		{"valid without photos", func(v *RoomVariant) {}, nil},
		{"valid local asset photo", func(v *RoomVariant) {
			v.Photos = []Photo{{URL: "/asset/rooms/uploads/x/abc.jpg", Alt: "Kamar"}}
		}, nil},
		{"valid https photo", func(v *RoomVariant) {
			v.Photos = []Photo{{URL: "https://cdn.example.com/a.jpg", Alt: "Kamar"}}
		}, nil},
		{"missing code", func(v *RoomVariant) { v.Code = " " }, ErrInvalidVariant},
		{"missing name", func(v *RoomVariant) { v.Name = "" }, ErrInvalidVariant},
		{"zero capacity", func(v *RoomVariant) { v.MaxCapacity = 0 }, ErrInvalidVariant},
		{"zero price", func(v *RoomVariant) { v.BasePriceMinor = 0 }, ErrInvalidVariant},
		{"too many photos", func(v *RoomVariant) { v.Photos = manyPhotos }, ErrInvalidVariant},
		{"empty photo url", func(v *RoomVariant) { v.Photos = []Photo{{URL: " ", Alt: "a"}} }, ErrInvalidVariant},
		{"javascript photo url", func(v *RoomVariant) { v.Photos = []Photo{{URL: "javascript:alert(1)", Alt: "a"}} }, ErrInvalidVariant},
		{"http (insecure) photo url", func(v *RoomVariant) { v.Photos = []Photo{{URL: "http://x.com/a.jpg", Alt: "a"}} }, ErrInvalidVariant},
		{"protocol relative photo url", func(v *RoomVariant) { v.Photos = []Photo{{URL: "//evil.com/a.jpg", Alt: "a"}} }, ErrInvalidVariant},
		{"path traversal photo url", func(v *RoomVariant) { v.Photos = []Photo{{URL: "/asset/rooms/../../etc/passwd", Alt: "a"}} }, ErrInvalidVariant},
		{"alt too long", func(v *RoomVariant) {
			v.Photos = []Photo{{URL: "/asset/rooms/bay/1.jpg", Alt: strings.Repeat("a", MaxPhotoAltLen+1)}}
		}, ErrInvalidVariant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validVariant()
			tt.mutate(&v)
			if got := ValidateVariant(v); !errors.Is(got, tt.want) {
				t.Errorf("ValidateVariant() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemoryStore_RejectsInvalidPhotos(t *testing.T) {
	store := NewMemoryStore(DefaultVariants())
	ctx := context.Background()
	bad := validVariant()
	bad.Photos = []Photo{{URL: "javascript:alert(1)", Alt: "a"}}

	if _, err := store.CreateVariant(ctx, bad); !errors.Is(err, ErrInvalidVariant) {
		t.Errorf("CreateVariant() err = %v, want ErrInvalidVariant", err)
	}
	if _, err := store.UpdateVariant(ctx, "sup-king", bad); !errors.Is(err, ErrInvalidVariant) {
		t.Errorf("UpdateVariant() err = %v, want ErrInvalidVariant", err)
	}
}
