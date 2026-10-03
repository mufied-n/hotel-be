package housekeeping

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type mockHousekeepingStore struct {
	rooms          map[string]*RoomOperationalView
	getErr         error
	updateErr      error
	listErr        error
	deductErr      error
	deductedRanges []struct {
		roomTypeID string
		start, end time.Time
	}
}

func newMockStore() *mockHousekeepingStore {
	return &mockHousekeepingStore{
		rooms: make(map[string]*RoomOperationalView),
	}
}

func (m *mockHousekeepingStore) GetRoom(ctx context.Context, roomNumber string) (*RoomOperationalView, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	r, ok := m.rooms[roomNumber]
	if !ok {
		return nil, ErrRoomNotFound
	}
	return r, nil
}

func (m *mockHousekeepingStore) UpdateRoomCleanliness(ctx context.Context, roomNumber string, to CleanlinessStatus, notes, actorID string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	r, ok := m.rooms[roomNumber]
	if !ok {
		return ErrRoomNotFound
	}
	r.CleanlinessStatus = to
	r.MaintenanceNotes = notes
	r.UpdatedBy = actorID
	r.UpdatedAt = time.Now()
	m.rooms[roomNumber] = r
	return nil
}

func (m *mockHousekeepingStore) ListRooms(ctx context.Context, floor int, status string, roomTypeID string) ([]RoomOperationalView, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var res []RoomOperationalView
	for _, r := range m.rooms {
		if (floor == 0 || r.Floor == floor) &&
			(status == "" || string(r.CleanlinessStatus) == status) &&
			(roomTypeID == "" || r.RoomTypeID == roomTypeID) {
			res = append(res, *r)
		}
	}
	return res, nil
}

func (m *mockHousekeepingStore) DeductInventoryForOOO(ctx context.Context, roomTypeID string, startDate, endDate time.Time) error {
	if m.deductErr != nil {
		return m.deductErr
	}
	m.deductedRanges = append(m.deductedRanges, struct {
		roomTypeID string
		start, end time.Time
	}{roomTypeID: roomTypeID, start: startDate, end: endDate})
	return nil
}

func TestHousekeepingService_UpdateStatus_TableTest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		initialStatus CleanlinessStatus
		input         UpdateStatusInput
		expectErr     error
		verifyStatus  CleanlinessStatus
	}{
		{
			name:          "Room attendant starts cleaning: vacant_dirty -> cleaning (Legal)",
			initialStatus: StatusVacantDirty,
			input: UpdateStatusInput{
				RoomNumber: "101",
				ToStatus:   StatusCleaning,
				Notes:      "Attendant started cleaning",
				ActorID:    "staff:hk_attendant_01",
				ActorRole:  "housekeeping",
			},
			expectErr:    nil,
			verifyStatus: StatusCleaning,
		},
		{
			name:          "Room attendant finishes cleaning: cleaning -> vacant_clean (Legal)",
			initialStatus: StatusCleaning,
			input: UpdateStatusInput{
				RoomNumber: "101",
				ToStatus:   StatusVacantClean,
				Notes:      "Linen replaced, bathroom sanitized",
				ActorID:    "staff:hk_attendant_01",
				ActorRole:  "housekeeping",
			},
			expectErr:    nil,
			verifyStatus: StatusVacantClean,
		},
		{
			name:          "HK Supervisor inspects: vacant_clean -> inspected (Legal)",
			initialStatus: StatusVacantClean,
			input: UpdateStatusInput{
				RoomNumber: "101",
				ToStatus:   StatusInspected,
				Notes:      "Quality check passed, amenities full",
				ActorID:    "staff:hk_supervisor_01",
				ActorRole:  "housekeeping",
			},
			expectErr:    nil,
			verifyStatus: StatusInspected,
		},
		{
			name:          "HK Supervisor rejects clean: vacant_clean -> vacant_dirty (Legal Rework)",
			initialStatus: StatusVacantClean,
			input: UpdateStatusInput{
				RoomNumber: "101",
				ToStatus:   StatusVacantDirty,
				Notes:      "Mirror stained, please clean again",
				ActorID:    "staff:hk_supervisor_01",
				ActorRole:  "housekeeping",
			},
			expectErr:    nil,
			verifyStatus: StatusVacantDirty,
		},
		{
			name:          "Attempt to skip cleaning: vacant_dirty -> inspected directly (Illegal)",
			initialStatus: StatusVacantDirty,
			input: UpdateStatusInput{
				RoomNumber: "101",
				ToStatus:   StatusInspected,
				Notes:      "Trying to jump directly to inspected",
				ActorID:    "staff:hk_attendant_01",
				ActorRole:  "housekeeping",
			},
			expectErr:    ErrInvalidTransition,
			verifyStatus: StatusVacantDirty,
		},
		{
			name:          "Non-GM staff attempts to set Out Of Order is rejected",
			initialStatus: StatusVacantDirty,
			input: UpdateStatusInput{
				RoomNumber: "101",
				ToStatus:   StatusOutOfOrder,
				Notes:      "AC broken",
				ActorID:    "staff:receptionist_01",
				ActorRole:  "receptionist",
			},
			expectErr:    ErrUnauthorizedTransition,
			verifyStatus: StatusVacantDirty,
		},
		{
			name:          "GM Admin sets Out Of Order (Legal)",
			initialStatus: StatusVacantDirty,
			input: UpdateStatusInput{
				RoomNumber: "101",
				ToStatus:   StatusOutOfOrder,
				Notes:      "Bathroom renovation scheduled",
				ActorID:    "staff:gm_01",
				ActorRole:  "gm_admin",
			},
			expectErr:    nil,
			verifyStatus: StatusOutOfOrder,
		},
		{
			name:          "Unknown room returns ErrRoomNotFound",
			initialStatus: StatusVacantDirty,
			input: UpdateStatusInput{
				RoomNumber: "999",
				ToStatus:   StatusCleaning,
				ActorRole:  "housekeeping",
			},
			expectErr: ErrRoomNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			store.rooms["101"] = &RoomOperationalView{
				RoomNumber:        "101",
				RoomTypeID:        "01900000-0000-7000-8000-000000000001",
				CleanlinessStatus: tc.initialStatus,
			}

			svc := NewService(store, slog.Default())
			err := svc.UpdateStatus(ctx, tc.input)

			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if store.rooms["101"].CleanlinessStatus != tc.verifyStatus {
				t.Errorf("status = %v, want %v", store.rooms["101"].CleanlinessStatus, tc.verifyStatus)
			}
		})
	}
}

func TestHousekeepingService_CheckInGuard_TableTest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		initialStatus CleanlinessStatus
		expectErr     error
		expectStatus  CleanlinessStatus
	}{
		{
			name:          "Inspected room is approved for check-in and marked occupied",
			initialStatus: StatusInspected,
			expectErr:     nil,
			expectStatus:  StatusOccupied,
		},
		{
			name:          "Vacant dirty room is blocked from check-in with ErrRoomNotReady",
			initialStatus: StatusVacantDirty,
			expectErr:     ErrRoomNotReady,
			expectStatus:  StatusVacantDirty,
		},
		{
			name:          "Cleaning in progress room is blocked from check-in with ErrRoomNotReady",
			initialStatus: StatusCleaning,
			expectErr:     ErrRoomNotReady,
			expectStatus:  StatusCleaning,
		},
		{
			name:          "Vacant clean room (not inspected yet) is blocked from check-in",
			initialStatus: StatusVacantClean,
			expectErr:     ErrRoomNotReady,
			expectStatus:  StatusVacantClean,
		},
		{
			name:          "Out of order room is blocked from check-in",
			initialStatus: StatusOutOfOrder,
			expectErr:     ErrRoomNotReady,
			expectStatus:  StatusOutOfOrder,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMockStore()
			store.rooms["201"] = &RoomOperationalView{
				RoomNumber:        "201",
				CleanlinessStatus: tc.initialStatus,
			}

			svc := NewService(store, slog.Default())
			err := svc.ValidateRoomForCheckIn(ctx, "201")

			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if store.rooms["201"].CleanlinessStatus != tc.expectStatus {
				t.Errorf("status = %v, want %v", store.rooms["201"].CleanlinessStatus, tc.expectStatus)
			}
		})
	}
}

func TestHousekeepingService_CheckOutAndOOO(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	store.rooms["301"] = &RoomOperationalView{
		RoomNumber:        "301",
		RoomTypeID:        "01900000-0000-7000-8000-000000000003",
		CleanlinessStatus: StatusOccupied,
	}

	svc := NewService(store, slog.Default())

	// 1. Mark dirty on check-out
	err := svc.MarkRoomDirtyOnCheckOut(ctx, "301")
	if err != nil {
		t.Fatalf("MarkRoomDirtyOnCheckOut failed: %v", err)
	}
	if store.rooms["301"].CleanlinessStatus != StatusVacantDirty {
		t.Errorf("status = %v, want vacant_dirty", store.rooms["301"].CleanlinessStatus)
	}

	// 2. Mark Out of Order with inventory deduction
	startDate := time.Now()
	endDate := startDate.Add(48 * time.Hour)
	err = svc.MarkRoomOutOfOrder(ctx, "301", startDate, endDate, "Major renovation")
	if err != nil {
		t.Fatalf("MarkRoomOutOfOrder failed: %v", err)
	}
	if store.rooms["301"].CleanlinessStatus != StatusOutOfOrder {
		t.Errorf("status = %v, want out_of_order", store.rooms["301"].CleanlinessStatus)
	}
	if len(store.deductedRanges) != 1 {
		t.Fatalf("expected 1 inventory deduction, got %d", len(store.deductedRanges))
	}
}

func TestHousekeepingService_GetRoomBoard(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	store.rooms["101"] = &RoomOperationalView{RoomNumber: "101", Floor: 1, CleanlinessStatus: StatusInspected}
	store.rooms["102"] = &RoomOperationalView{RoomNumber: "102", Floor: 1, CleanlinessStatus: StatusVacantDirty}
	store.rooms["201"] = &RoomOperationalView{RoomNumber: "201", Floor: 2, CleanlinessStatus: StatusOccupied}

	svc := NewService(store, slog.Default())

	board, err := svc.GetRoomBoard(ctx, 0, "", "")
	if err != nil {
		t.Fatalf("GetRoomBoard failed: %v", err)
	}
	if board.TotalRooms != 3 {
		t.Errorf("total rooms = %d, want 3", board.TotalRooms)
	}
	if board.Summary[StatusInspected] != 1 || board.Summary[StatusVacantDirty] != 1 || board.Summary[StatusOccupied] != 1 {
		t.Errorf("unexpected summary breakdown: %+v", board.Summary)
	}
}
