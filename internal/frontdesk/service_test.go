package frontdesk

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockStore struct {
	dailyRosterFunc     func(ctx context.Context, targetDate time.Time) (*DailyRoster, error)
	createHandoverFunc  func(ctx context.Context, note *HandoverNote) error
	listHandoversFunc   func(ctx context.Context, limit, offset int) ([]HandoverNote, int, error)
}

func (m *mockStore) GetDailyRoster(ctx context.Context, targetDate time.Time) (*DailyRoster, error) {
	if m.dailyRosterFunc != nil {
		return m.dailyRosterFunc(ctx, targetDate)
	}
	return &DailyRoster{
		Date: targetDate.Format("2006-01-02"),
		Metrics: RosterMetrics{
			TotalRooms:           95,
			SellableRooms:        94,
			OutOfOrderRooms:      1,
			OccupiedRooms:        60,
			VacantInspectedRooms: 20,
			VacantDirtyRooms:     10,
			CleaningRooms:        4,
			OccupancyRatePercent: 63.83,
		},
		InHouseCount: 60,
	}, nil
}

func (m *mockStore) CreateHandoverNote(ctx context.Context, note *HandoverNote) error {
	if m.createHandoverFunc != nil {
		return m.createHandoverFunc(ctx, note)
	}
	note.ID = "note-mock-001"
	return nil
}

func (m *mockStore) ListHandoverNotes(ctx context.Context, limit, offset int) ([]HandoverNote, int, error) {
	if m.listHandoversFunc != nil {
		return m.listHandoversFunc(ctx, limit, offset)
	}
	return []HandoverNote{
		{
			ID:             "note-mock-001",
			Shift:          ShiftMorning,
			CashFloatMinor: 1500000,
			PendingIssues:  "None",
			ActorID:        "staff:receptionist_01",
			CreatedAt:      time.Now(),
		},
	}, 1, nil
}

func TestFrontDeskService_RecordHandover_TableTest(t *testing.T) {
	tests := []struct {
		name        string
		input       RecordHandoverInput
		mockErr     error
		expectErr   error
		expectShift ShiftType
	}{
		{
			name: "Valid morning shift handover",
			input: RecordHandoverInput{
				Shift:          ShiftMorning,
				CashFloatMinor: 1500000,
				PendingIssues:  "AC repair in 205",
				VIPGuestNotes:  "Airport taxi for room 501",
				ActorID:        "staff:ryan",
				ActorRole:      "receptionist",
			},
			mockErr:     nil,
			expectErr:   nil,
			expectShift: ShiftMorning,
		},
		{
			name: "Valid afternoon shift handover with uppercase letters",
			input: RecordHandoverInput{
				Shift:          "AFTERNOON",
				CashFloatMinor: 1750000,
				PendingIssues:  "Late checkout approved for 302",
			},
			mockErr:     nil,
			expectErr:   nil,
			expectShift: ShiftAfternoon,
		},
		{
			name: "Valid night shift handover",
			input: RecordHandoverInput{
				Shift:          ShiftNight,
				CashFloatMinor: 1500000,
				PendingIssues:  "Night audit completed cleanly",
			},
			mockErr:     nil,
			expectErr:   nil,
			expectShift: ShiftNight,
		},
		{
			name: "Invalid shift string returns ErrInvalidShift",
			input: RecordHandoverInput{
				Shift: "midday",
			},
			expectErr: ErrInvalidShift,
		},
		{
			name: "Store failure returns wrapped error",
			input: RecordHandoverInput{
				Shift:         ShiftMorning,
				PendingIssues: "AC issues in room 201",
			},
			mockErr:   errors.New("db disk failure"),
			expectErr: errors.New("db disk failure"),
		},
		{
			name: "Empty handover notes return ErrEmptyHandoverNote",
			input: RecordHandoverInput{
				Shift:         ShiftMorning,
				PendingIssues: "",
				VIPGuestNotes: "",
			},
			expectErr: ErrEmptyHandoverNote,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{
				createHandoverFunc: func(ctx context.Context, note *HandoverNote) error {
					if tc.mockErr != nil {
						return tc.mockErr
					}
					note.ID = "generated-uuid"
					return nil
				},
			}

			svc := NewService(store, nil)
			res, err := svc.RecordHandover(context.Background(), tc.input)

			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, tc.expectErr) && err.Error() != tc.expectErr.Error() && !contains(err.Error(), tc.expectErr.Error()) {
					t.Fatalf("expected error %v, got %v", tc.expectErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Shift != tc.expectShift {
				t.Errorf("expected shift %s, got %s", tc.expectShift, res.Shift)
			}
			if res.ID == "" {
				t.Errorf("expected generated ID, got empty")
			}
		})
	}
}

func TestFrontDeskService_GetDailyRoster_TableTest(t *testing.T) {
	tests := []struct {
		name       string
		targetDate time.Time
		expectDate string
	}{
		{
			name:       "Specific target date",
			targetDate: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
			expectDate: "2026-10-05",
		},
		{
			name:       "Zero date fallback to today",
			targetDate: time.Time{},
			expectDate: time.Now().Format("2006-01-02"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{}
			svc := NewService(store, nil)

			res, err := svc.GetDailyRoster(context.Background(), tc.targetDate)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Date != tc.expectDate {
				t.Errorf("expected date %s, got %s", tc.expectDate, res.Date)
			}
			if res.Metrics.TotalRooms != 95 {
				t.Errorf("expected 95 total rooms, got %d", res.Metrics.TotalRooms)
			}
		})
	}
}

func TestFrontDeskService_ListHandovers(t *testing.T) {
	store := &mockStore{}
	svc := NewService(store, nil)

	notes, total, err := svc.ListHandovers(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 || len(notes) != 1 {
		t.Errorf("expected 1 note, got total %d, len %d", total, len(notes))
	}
	if notes[0].Shift != ShiftMorning {
		t.Errorf("expected morning shift, got %s", notes[0].Shift)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
