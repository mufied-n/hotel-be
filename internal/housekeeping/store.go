package housekeeping

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrRoomNotFound dikembalikan jika nomor kamar fisik tidak terdaftar.
	ErrRoomNotFound = errors.New("housekeeping: kamar tidak ditemukan")
	// ErrInvalidTransition dikembalikan jika perpindahan status melanggar aturan state machine.
	ErrInvalidTransition = errors.New("housekeeping: transisi status kamar tidak valid")
	// ErrRoomNotReady dikembalikan saat check-in ke kamar yang belum inspected.
	ErrRoomNotReady = errors.New("housekeeping: kamar belum berstatus inspected (siap huni)")
	// ErrUnauthorizedTransition dikembalikan jika role tidak berhak mengubah ke status target.
	ErrUnauthorizedTransition = errors.New("housekeeping: peran staf tidak memiliki izin untuk transisi status ini")
)

// Store mendefinisikan port penyimpanan data kamar dan inventaris.
type Store interface {
	GetRoom(ctx context.Context, roomNumber string) (*RoomOperationalView, error)
	UpdateRoomCleanliness(ctx context.Context, roomNumber string, to CleanlinessStatus, notes, actorID string) error
	ListRooms(ctx context.Context, floor int, status string, roomTypeID string) ([]RoomOperationalView, error)
	DeductInventoryForOOO(ctx context.Context, roomTypeID string, startDate, endDate time.Time) error
}
