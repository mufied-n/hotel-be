package assistance

import (
	"context"
	"time"
)

// Store mendefinisikan kontrak persistensi data permintaan khusus.
type Store interface {
	CreateRequest(ctx context.Context, req SpecialRequest) (*SpecialRequest, error)
	GetRequestByID(ctx context.Context, id string) (*SpecialRequest, error)
	ListByBookingID(ctx context.Context, bookingID string) ([]SpecialRequest, error)
	ListStaffQueue(ctx context.Context, filter ListFilter) ([]StaffQueueItem, error)
	UpdateStatus(ctx context.Context, reqID string, toStatus Status, notes string, handledBy string, handledAt time.Time) (*SpecialRequest, error)
	GetBookingOwner(ctx context.Context, bookingID string) (guestEmail string, exists bool, err error)
}
