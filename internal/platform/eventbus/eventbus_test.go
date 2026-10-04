package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func startEmbeddedNATS(t *testing.T) (*natsserver.Server, string) {
	t.Helper()
	opts := &natsserver.Options{
		Port:      -1, // random ephemeral port
		JetStream: true,
		NoLog:     true,
		NoSigs:    true,
	}
	s, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create embedded nats server: %v", err)
	}
	go s.Start()
	if !s.ReadyForConnections(4 * time.Second) {
		t.Fatal("embedded nats server did not become ready in time")
	}
	return s, s.ClientURL()
}

func TestMemoryBus_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		subPattern   string
		pubSubject   string
		eventType    string
		eventID      string
		payload      []byte
		expectMatch  bool
	}{
		{
			name:        "Exact match on subject",
			subPattern:  "hospitality.booking.b1.confirmed",
			pubSubject:  "hospitality.booking.b1.confirmed",
			eventType:   "payment_confirmed",
			eventID:     "evt-01",
			payload:     []byte(`{"status":"CONFIRMED"}`),
			expectMatch: true,
		},
		{
			name:        "Single-token wildcard match",
			subPattern:  "hospitality.booking.*.confirmed",
			pubSubject:  "hospitality.booking.b123.confirmed",
			eventType:   "payment_confirmed",
			eventID:     "evt-02",
			payload:     []byte(`{"status":"CONFIRMED"}`),
			expectMatch: true,
		},
		{
			name:        "Multi-token wildcard match with greater-than",
			subPattern:  "hospitality.booking.>",
			pubSubject:  "hospitality.booking.b99.special.vip.confirmed",
			eventType:   "vip_booking",
			eventID:     "evt-03",
			payload:     []byte(`{"vip":true}`),
			expectMatch: true,
		},
		{
			name:        "Mismatched subject returns false",
			subPattern:  "hospitality.booking.*.confirmed",
			pubSubject:  "hospitality.room.r101.status_changed",
			eventType:   "room_cleaned",
			eventID:     "evt-04",
			payload:     []byte(`{"status":"CLEAN"}`),
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := NewMemoryBus()
			defer bus.Close()

			received := make(chan *EventMsg, 1)
			sub, err := bus.Subscribe(tt.subPattern, func(msg *EventMsg) {
				received <- msg
			})
			if err != nil {
				t.Fatalf("Subscribe failed: %v", err)
			}
			defer sub.Unsubscribe()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err = bus.Publish(ctx, tt.pubSubject, tt.eventType, tt.eventID, tt.payload)
			if err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			if tt.expectMatch {
				select {
				case msg := <-received:
					if msg.ID != tt.eventID {
						t.Errorf("expected ID %s, got %s", tt.eventID, msg.ID)
					}
					if msg.EventType != tt.eventType {
						t.Errorf("expected EventType %s, got %s", tt.eventType, msg.EventType)
					}
				case <-time.After(500 * time.Millisecond):
					t.Errorf("timed out waiting for expected message")
				}
			} else {
				select {
				case msg := <-received:
					t.Errorf("unexpected message received: %+v", msg)
				case <-time.After(100 * time.Millisecond):
					// Expected no message
				}
			}
		})
	}
}

func TestMemoryBus_SubscribeChanAndUnsubscribe(t *testing.T) {
	bus := NewMemoryBus()
	defer bus.Close()

	ch := make(chan *EventMsg, 10)
	sub, err := bus.SubscribeChan("hospitality.booking.*", ch)
	if err != nil {
		t.Fatalf("SubscribeChan failed: %v", err)
	}

	ctx := context.Background()
	_ = bus.Publish(ctx, "hospitality.booking.1", "created", "e1", []byte("data1"))

	select {
	case msg := <-ch:
		if msg.ID != "e1" {
			t.Errorf("expected ID e1, got %s", msg.ID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected message on channel")
	}

	// Unsubscribe
	_ = sub.Unsubscribe()
	_ = bus.Publish(ctx, "hospitality.booking.2", "created", "e2", []byte("data2"))

	select {
	case msg := <-ch:
		t.Fatalf("unexpected message after unsubscribe: %+v", msg)
	case <-time.After(100 * time.Millisecond):
		// Expected
	}
}

func TestNATSBus_TableDriven(t *testing.T) {
	server, clientURL := startEmbeddedNATS(t)
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bus, err := NewNATSBus(ctx, clientURL)
	if err != nil {
		t.Fatalf("NewNATSBus failed: %v", err)
	}
	defer bus.Close()

	tests := []struct {
		name       string
		subPattern string
		pubSubject string
		eventType  string
		eventID    string
		payload    []byte
	}{
		{
			name:       "Single booking event over NATS JetStream",
			subPattern: "hospitality.booking.b100.*",
			pubSubject: "hospitality.booking.b100.confirmed",
			eventType:  "payment_confirmed",
			eventID:    "nats-evt-01",
			payload:    []byte(`{"booking_id":"b100","status":"CONFIRMED"}`),
		},
		{
			name:       "Front desk wildcard event over NATS JetStream",
			subPattern: "hospitality.booking.>",
			pubSubject: "hospitality.booking.b101.created",
			eventType:  "booking_created",
			eventID:    "nats-evt-02",
			payload:    []byte(`{"booking_id":"b101","status":"CREATED"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			received := make(chan *EventMsg, 1)
			sub, err := bus.Subscribe(tt.subPattern, func(msg *EventMsg) {
				received <- msg
			})
			if err != nil {
				t.Fatalf("Subscribe failed: %v", err)
			}
			defer sub.Unsubscribe()

			// Wait for subscription registration
			time.Sleep(50 * time.Millisecond)

			err = bus.Publish(ctx, tt.pubSubject, tt.eventType, tt.eventID, tt.payload)
			if err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			select {
			case msg := <-received:
				if msg.ID != tt.eventID {
					t.Errorf("expected ID %s, got %s", tt.eventID, msg.ID)
				}
				if msg.EventType != tt.eventType {
					t.Errorf("expected EventType %s, got %s", tt.eventType, msg.EventType)
				}
				if string(msg.Payload) != string(tt.payload) {
					t.Errorf("expected payload %s, got %s", string(tt.payload), string(msg.Payload))
				}
			case <-time.After(1 * time.Second):
				t.Fatal("timeout waiting for NATS message")
			}
		})
	}
}

func TestNATSBus_SubscribeChan(t *testing.T) {
	server, clientURL := startEmbeddedNATS(t)
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bus, err := NewNATSBus(ctx, clientURL)
	if err != nil {
		t.Fatalf("NewNATSBus failed: %v", err)
	}
	defer bus.Close()

	ch := make(chan *EventMsg, 10)
	sub, err := bus.SubscribeChan("hospitality.booking.chan.*", ch)
	if err != nil {
		t.Fatalf("SubscribeChan failed: %v", err)
	}
	defer sub.Unsubscribe()

	time.Sleep(50 * time.Millisecond)

	err = bus.Publish(ctx, "hospitality.booking.chan.test", "chan_event", "chan-evt-01", []byte("channel-payload"))
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case msg := <-ch:
		if msg.ID != "chan-evt-01" {
			t.Errorf("expected ID chan-evt-01, got %s", msg.ID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting on SubscribeChan")
	}
}

func TestNATSBus_Concurrency(t *testing.T) {
	server, clientURL := startEmbeddedNATS(t)
	defer server.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bus, err := NewNATSBus(ctx, clientURL)
	if err != nil {
		t.Fatalf("NewNATSBus failed: %v", err)
	}
	defer bus.Close()

	var wg sync.WaitGroup
	receivedCount := make(chan int, 50)

	sub, err := bus.Subscribe("hospitality.concurrent.>", func(msg *EventMsg) {
		receivedCount <- 1
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Unsubscribe()

	time.Sleep(50 * time.Millisecond)

	numEvents := 20
	for i := 0; i < numEvents; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			_ = bus.Publish(ctx, "hospitality.concurrent.evt", "concurrent_test", time.Now().String(), []byte{byte(idx)})
		}()
	}
	wg.Wait()

	received := 0
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-receivedCount:
			received++
			if received >= numEvents {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if received < numEvents {
		t.Errorf("expected %d events, received %d", numEvents, received)
	}
}
