package eventbus

import (
	"context"
	"time"
)

// EventMsg merepresentasikan pesan yang dialirkan dalam event fabric.
type EventMsg struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`
	EventType string    `json:"event_type"`
	Payload   []byte    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

// Subscription adalah antarmuka untuk membatalkan langganan event stream.
type Subscription interface {
	Unsubscribe() error
}

// Bus adalah antarmuka generik event bus untuk real-time pub/sub dan streaming.
type Bus interface {
	// Publish mengirimkan event ke subjek tertentu dengan dukungan deduplikasi melalui eventID.
	Publish(ctx context.Context, subject, eventType, eventID string, payload []byte) error

	// Subscribe mendaftarkan handler fungsi untuk subjek tertentu (mendukung wildcard * dan >).
	Subscribe(subject string, handler func(msg *EventMsg)) (Subscription, error)

	// SubscribeChan mendaftarkan channel Go untuk menerima pesan langsung dari stream.
	SubscribeChan(subject string, ch chan *EventMsg) (Subscription, error)

	// Close menutup koneksi bus.
	Close() error
}
