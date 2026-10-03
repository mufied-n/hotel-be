package frontdesk

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidShift dikembalikan bila nilai shift bukan morning, afternoon, atau night.
	ErrInvalidShift = errors.New("frontdesk: shift harus berupa 'morning', 'afternoon', atau 'night'")
	// ErrEmptyHandoverNote dikembalikan bila pending issues dan catatan penting kosong.
	ErrEmptyHandoverNote = errors.New("frontdesk: catatan serah terima shift tidak boleh kosong")
)

// Store mendefinisikan port driven untuk agregasi data dan persistensi meja depan.
type Store interface {
	GetDailyRoster(ctx context.Context, targetDate time.Time) (*DailyRoster, error)
	CreateHandoverNote(ctx context.Context, note *HandoverNote) error
	ListHandoverNotes(ctx context.Context, limit, offset int) ([]HandoverNote, int, error)
}
