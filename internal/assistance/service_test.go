package assistance

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockStore struct {
	createRequestFn   func(ctx context.Context, req SpecialRequest) (*SpecialRequest, error)
	getRequestByIDFn  func(ctx context.Context, id string) (*SpecialRequest, error)
	listByBookingIDFn func(ctx context.Context, bookingID string) ([]SpecialRequest, error)
	listStaffQueueFn  func(ctx context.Context, filter ListFilter) ([]StaffQueueItem, error)
	updateStatusFn    func(ctx context.Context, reqID string, toStatus Status, notes string, handledBy string, handledAt time.Time) (*SpecialRequest, error)
	getBookingOwnerFn func(ctx context.Context, bookingID string) (guestEmail string, exists bool, err error)
}

func (m *mockStore) CreateRequest(ctx context.Context, req SpecialRequest) (*SpecialRequest, error) {
	if m.createRequestFn != nil {
		return m.createRequestFn(ctx, req)
	}
	return &req, nil
}

func (m *mockStore) GetRequestByID(ctx context.Context, id string) (*SpecialRequest, error) {
	if m.getRequestByIDFn != nil {
		return m.getRequestByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockStore) ListByBookingID(ctx context.Context, bookingID string) ([]SpecialRequest, error) {
	if m.listByBookingIDFn != nil {
		return m.listByBookingIDFn(ctx, bookingID)
	}
	return nil, nil
}

func (m *mockStore) ListStaffQueue(ctx context.Context, filter ListFilter) ([]StaffQueueItem, error) {
	if m.listStaffQueueFn != nil {
		return m.listStaffQueueFn(ctx, filter)
	}
	return nil, nil
}

func (m *mockStore) UpdateStatus(ctx context.Context, reqID string, toStatus Status, notes string, handledBy string, handledAt time.Time) (*SpecialRequest, error) {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, reqID, toStatus, notes, handledBy, handledAt)
	}
	return nil, nil
}

func (m *mockStore) GetBookingOwner(ctx context.Context, bookingID string) (guestEmail string, exists bool, err error) {
	if m.getBookingOwnerFn != nil {
		return m.getBookingOwnerFn(ctx, bookingID)
	}
	return "", false, nil
}

func TestService_CreateGuestRequest_TableDriven(t *testing.T) {
	fixedTime := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	testBookingID := "01900000-0000-7000-8000-000000000001"
	testGuestEmail := "budi@example.com"

	tests := []struct {
		name           string
		guestEmail     string
		input          CreateRequestInput
		mockOwnerEmail string
		mockExists     bool
		mockOwnerErr   error
		mockCreateErr  error
		wantErr        error
		wantDept       Department
	}{
		{
			name:       "Success - Housekeeping celebration setup auto-routing",
			guestEmail: testGuestEmail,
			input: CreateRequestInput{
				BookingID:   testBookingID,
				Category:    CategoryCelebrationSetup,
				Description: "Anniversary handuk angsa",
				TargetTime:  "14:00",
			},
			mockOwnerEmail: testGuestEmail,
			mockExists:     true,
			wantErr:        nil,
			wantDept:       DepartmentHousekeeping,
		},
		{
			name:       "Success - Front Desk early arrival auto-routing",
			guestEmail: testGuestEmail,
			input: CreateRequestInput{
				BookingID:   testBookingID,
				Category:    CategoryEarlyArrival,
				Description: "Tiba jam 11 siang mohon check in awal",
				TargetTime:  "11:00",
			},
			mockOwnerEmail: testGuestEmail,
			mockExists:     true,
			wantErr:        nil,
			wantDept:       DepartmentFrontDesk,
		},
		{
			name:       "Fail - Empty Booking ID",
			guestEmail: testGuestEmail,
			input: CreateRequestInput{
				BookingID:   "",
				Category:    CategoryCelebrationSetup,
				Description: "Dekorasi kamar",
			},
			wantErr: ErrBookingNotFound,
		},
		{
			name:       "Fail - Invalid Category",
			guestEmail: testGuestEmail,
			input: CreateRequestInput{
				BookingID:   testBookingID,
				Category:    Category("invalid_category"),
				Description: "Dekorasi kamar",
			},
			wantErr: ErrInvalidCategory,
		},
		{
			name:       "Fail - Empty Description",
			guestEmail: testGuestEmail,
			input: CreateRequestInput{
				BookingID:   testBookingID,
				Category:    CategoryBabyCrib,
				Description: "   ",
			},
			wantErr: ErrEmptyDescription,
		},
		{
			name:       "Fail - DB Error on GetBookingOwner",
			guestEmail: testGuestEmail,
			input: CreateRequestInput{
				BookingID:   testBookingID,
				Category:    CategoryBabyCrib,
				Description: "Baby crib untuk balita",
			},
			mockOwnerErr: errors.New("db error"),
			wantErr:      errors.New("db error"),
		},
		{
			name:       "Fail - Booking Not Found",
			guestEmail: testGuestEmail,
			input: CreateRequestInput{
				BookingID:   testBookingID,
				Category:    CategoryBabyCrib,
				Description: "Baby crib untuk balita",
			},
			mockExists: false,
			wantErr:    ErrBookingNotFound,
		},
		{
			name:       "Fail - Anti-IDOR Defense (different guest email)",
			guestEmail: "attacker@evil.com",
			input: CreateRequestInput{
				BookingID:   testBookingID,
				Category:    CategoryBabyCrib,
				Description: "Baby crib untuk balita",
			},
			mockOwnerEmail: testGuestEmail,
			mockExists:     true,
			wantErr:        ErrBookingNotFound,
		},
		{
			name:       "Fail - Store Create Error",
			guestEmail: testGuestEmail,
			input: CreateRequestInput{
				BookingID:   testBookingID,
				Category:    CategoryDietaryAllergy,
				Description: "Alergi kacang tanah",
			},
			mockOwnerEmail: testGuestEmail,
			mockExists:     true,
			mockCreateErr:  errors.New("store insert failure"),
			wantErr:        errors.New("store insert failure"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{
				getBookingOwnerFn: func(ctx context.Context, bookingID string) (string, bool, error) {
					return tc.mockOwnerEmail, tc.mockExists, tc.mockOwnerErr
				},
				createRequestFn: func(ctx context.Context, req SpecialRequest) (*SpecialRequest, error) {
					if tc.mockCreateErr != nil {
						return nil, tc.mockCreateErr
					}
					req.ID = "req-123"
					return &req, nil
				},
			}

			svc := NewService(store, nil)
			svc.SetClock(func() time.Time { return fixedTime })

			res, err := svc.CreateGuestRequest(context.Background(), tc.guestEmail, tc.input)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.wantErr)
				}
				if !errors.Is(err, tc.wantErr) && err.Error() != tc.wantErr.Error() {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Department != tc.wantDept {
				t.Errorf("expected department %s, got %s", tc.wantDept, res.Department)
			}
			if res.Status != StatusPending {
				t.Errorf("expected status pending, got %s", res.Status)
			}
		})
	}
}

func TestService_ListGuestRequests_TableDriven(t *testing.T) {
	testBookingID := "01900000-0000-7000-8000-000000000001"
	testGuestEmail := "budi@example.com"

	tests := []struct {
		name           string
		guestEmail     string
		bookingID      string
		mockOwnerEmail string
		mockExists     bool
		mockOwnerErr   error
		wantErr        error
		wantCount      int
	}{
		{
			name:           "Success - List requests",
			guestEmail:     testGuestEmail,
			bookingID:      testBookingID,
			mockOwnerEmail: testGuestEmail,
			mockExists:     true,
			wantErr:        nil,
			wantCount:      2,
		},
		{
			name:       "Fail - Empty booking ID",
			guestEmail: testGuestEmail,
			bookingID:  "",
			wantErr:    ErrBookingNotFound,
		},
		{
			name:         "Fail - DB Error on owner lookup",
			guestEmail:   testGuestEmail,
			bookingID:    testBookingID,
			mockOwnerErr: errors.New("db error"),
			wantErr:      errors.New("db error"),
		},
		{
			name:           "Fail - Anti-IDOR booking owner mismatch",
			guestEmail:     "other@example.com",
			bookingID:      testBookingID,
			mockOwnerEmail: testGuestEmail,
			mockExists:     true,
			wantErr:        ErrBookingNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{
				getBookingOwnerFn: func(ctx context.Context, bookingID string) (string, bool, error) {
					return tc.mockOwnerEmail, tc.mockExists, tc.mockOwnerErr
				},
				listByBookingIDFn: func(ctx context.Context, bookingID string) ([]SpecialRequest, error) {
					return make([]SpecialRequest, tc.wantCount), nil
				},
			}

			svc := NewService(store, nil)
			items, err := svc.ListGuestRequests(context.Background(), tc.guestEmail, tc.bookingID)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tc.wantCount {
				t.Errorf("expected %d items, got %d", tc.wantCount, len(items))
			}
		})
	}
}

func TestService_ListStaffQueue_TableDriven(t *testing.T) {
	store := &mockStore{
		listStaffQueueFn: func(ctx context.Context, filter ListFilter) ([]StaffQueueItem, error) {
			return []StaffQueueItem{
				{
					SpecialRequest: SpecialRequest{ID: "req-1", Category: CategoryBabyCrib, Department: DepartmentHousekeeping},
					GuestName:      "Siti Rahayu",
				},
			}, nil
		},
	}

	svc := NewService(store, nil)
	items, err := svc.ListStaffQueue(context.Background(), ListFilter{Department: "housekeeping"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].GuestName != "Siti Rahayu" {
		t.Errorf("unexpected items returned: %+v", items)
	}
}

func TestService_UpdateStatus_TableDriven(t *testing.T) {
	fixedTime := time.Date(2026, 10, 3, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name          string
		input         UpdateStatusInput
		mockExisting  *SpecialRequest
		mockFetchErr  error
		mockUpdateErr error
		wantErr       error
		wantStatus    Status
	}{
		{
			name: "Success - pending to acknowledged",
			input: UpdateStatusInput{
				RequestID:  "req-1",
				ToStatus:   StatusAcknowledged,
				StaffNotes: "Diterima dan dicatat",
				HandledBy:  "receptionist",
			},
			mockExisting: &SpecialRequest{ID: "req-1", Status: StatusPending},
			wantErr:      nil,
			wantStatus:   StatusAcknowledged,
		},
		{
			name: "Success - acknowledged to fulfilled",
			input: UpdateStatusInput{
				RequestID:  "req-1",
				ToStatus:   StatusFulfilled,
				StaffNotes: "Handuk angsa sudah siap di kamar",
				HandledBy:  "housekeeping",
			},
			mockExisting: &SpecialRequest{ID: "req-1", Status: StatusAcknowledged},
			wantErr:      nil,
			wantStatus:   StatusFulfilled,
		},
		{
			name: "Success - pending to declined with reason",
			input: UpdateStatusInput{
				RequestID:  "req-1",
				ToStatus:   StatusDeclined,
				StaffNotes: "Mohon maaf kamar lantai atas telah penuh terisi",
				HandledBy:  "receptionist",
			},
			mockExisting: &SpecialRequest{ID: "req-1", Status: StatusPending},
			wantErr:      nil,
			wantStatus:   StatusDeclined,
		},
		{
			name: "Fail - Empty Request ID",
			input: UpdateStatusInput{
				RequestID: "",
				ToStatus:  StatusAcknowledged,
			},
			wantErr: ErrRequestNotFound,
		},
		{
			name: "Fail - Invalid Status",
			input: UpdateStatusInput{
				RequestID: "req-1",
				ToStatus:  Status("unknown_status"),
			},
			wantErr: ErrInvalidStatusTransition,
		},
		{
			name: "Fail - Cannot transition to pending",
			input: UpdateStatusInput{
				RequestID: "req-1",
				ToStatus:  StatusPending,
			},
			wantErr: ErrInvalidStatusTransition,
		},
		{
			name: "Fail - DB Fetch Error",
			input: UpdateStatusInput{
				RequestID: "req-1",
				ToStatus:  StatusAcknowledged,
			},
			mockFetchErr: errors.New("db fetch error"),
			wantErr:      errors.New("db fetch error"),
		},
		{
			name: "Fail - Request Not Found",
			input: UpdateStatusInput{
				RequestID: "req-1",
				ToStatus:  StatusAcknowledged,
			},
			mockExisting: nil,
			wantErr:      ErrRequestNotFound,
		},
		{
			name: "Fail - Transition from terminal fulfilled status",
			input: UpdateStatusInput{
				RequestID: "req-1",
				ToStatus:  StatusAcknowledged,
			},
			mockExisting: &SpecialRequest{ID: "req-1", Status: StatusFulfilled},
			wantErr:      ErrInvalidStatusTransition,
		},
		{
			name: "Fail - Transition from terminal declined status",
			input: UpdateStatusInput{
				RequestID: "req-1",
				ToStatus:  StatusFulfilled,
			},
			mockExisting: &SpecialRequest{ID: "req-1", Status: StatusDeclined},
			wantErr:      ErrInvalidStatusTransition,
		},
		{
			name: "Fail - Declined without required notes (notes < 5 chars)",
			input: UpdateStatusInput{
				RequestID:  "req-1",
				ToStatus:   StatusDeclined,
				StaffNotes: "no",
			},
			mockExisting: &SpecialRequest{ID: "req-1", Status: StatusPending},
			wantErr:      ErrStaffNotesRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{
				getRequestByIDFn: func(ctx context.Context, id string) (*SpecialRequest, error) {
					return tc.mockExisting, tc.mockFetchErr
				},
				updateStatusFn: func(ctx context.Context, reqID string, toStatus Status, notes, handledBy string, handledAt time.Time) (*SpecialRequest, error) {
					if tc.mockUpdateErr != nil {
						return nil, tc.mockUpdateErr
					}
					return &SpecialRequest{
						ID:         reqID,
						Status:     toStatus,
						StaffNotes: notes,
						HandledBy:  handledBy,
						HandledAt:  &handledAt,
					}, nil
				},
			}

			svc := NewService(store, nil)
			svc.SetClock(func() time.Time { return fixedTime })

			res, err := svc.UpdateStatus(context.Background(), tc.input)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.wantErr)
				}
				if !errors.Is(err, tc.wantErr) && err.Error() != tc.wantErr.Error() {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Status != tc.wantStatus {
				t.Errorf("expected status %s, got %s", tc.wantStatus, res.Status)
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	// IsValidCategory
	if !IsValidCategory(CategoryCelebrationSetup) {
		t.Errorf("expected valid for celebration_setup")
	}
	if IsValidCategory(Category("bogus")) {
		t.Errorf("expected invalid for bogus")
	}

	// IsValidStatus
	if !IsValidStatus(StatusFulfilled) {
		t.Errorf("expected valid for fulfilled")
	}
	if IsValidStatus(Status("bogus")) {
		t.Errorf("expected invalid for bogus")
	}

	// ResolveDepartment
	if ResolveDepartment(CategoryBabyCrib) != DepartmentHousekeeping {
		t.Errorf("expected housekeeping for baby_crib")
	}
	if ResolveDepartment(CategoryLateDeparture) != DepartmentFrontDesk {
		t.Errorf("expected front_desk for late_departure")
	}
	if ResolveDepartment(Category("unknown")) != DepartmentFrontDesk {
		t.Errorf("expected default front_desk for unknown")
	}

	// CleanString
	if CleanString("  hello  ") != "hello" {
		t.Errorf("expected 'hello'")
	}
}
