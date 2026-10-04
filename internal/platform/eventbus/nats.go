package eventbus

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	StreamName     = "HOSPITALITY_EVENTS"
	DefaultSubject = "hospitality.>"
)

type natsSub struct {
	sub *nats.Subscription
}

func (s *natsSub) Unsubscribe() error {
	if s.sub != nil {
		return s.sub.Unsubscribe()
	}
	return nil
}

// NATSBus adalah implementasi Bus berbasis NATS JetStream berkinerja tinggi.
type NATSBus struct {
	nc *nats.Conn
	js jetstream.JetStream
}

// NewNATSBus menghubungkan ke NATS server dan menginisialisasi JetStream stream.
func NewNATSBus(ctx context.Context, natsURL string) (*NATSBus, error) {
	opts := []nats.Option{
		nats.Name("pulang-hotel-booking-engine"),
		nats.Timeout(5 * time.Second),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // Terus mencoba rekoneksi
	}

	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats.Connect failed: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream.New failed: %w", err)
	}

	// Buat atau perbarui stream terpusat
	streamCfg := jetstream.StreamConfig{
		Name:        StreamName,
		Description: "Pulang ke Uttara Real-Time Hospitality Events",
		Subjects:    []string{DefaultSubject},
		Storage:     jetstream.FileStorage,
		Retention:   jetstream.LimitsPolicy,
		Discard:     jetstream.DiscardOld,
		MaxAge:      7 * 24 * time.Hour,
		Duplicates:  5 * time.Minute, // Server-side deduplication window
	}

	// Gunakan MemoryStorage jika FileStorage gagal (misal di ephemeral / in-memory server)
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(initCtx, streamCfg)
	if err != nil {
		// Fallback ke MemoryStorage jika diuji tanpa volume disk
		streamCfg.Storage = jetstream.MemoryStorage
		_, err = js.CreateOrUpdateStream(initCtx, streamCfg)
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("CreateOrUpdateStream failed: %w", err)
		}
	}

	return &NATSBus{
		nc: nc,
		js: js,
	}, nil
}

func (b *NATSBus) Publish(ctx context.Context, subject, eventType, eventID string, payload []byte) error {
	msg := nats.NewMsg(subject)
	msg.Header.Set("Nats-Msg-Id", eventID) // Deduplikasi bawaan JetStream
	msg.Header.Set("Event-Type", eventType)
	msg.Header.Set("Timestamp", time.Now().UTC().Format(time.RFC3339))
	msg.Data = payload

	_, err := b.js.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("js.PublishMsg failed: %w", err)
	}
	return nil
}

func (b *NATSBus) Subscribe(subject string, handler func(msg *EventMsg)) (Subscription, error) {
	sub, err := b.nc.Subscribe(subject, func(m *nats.Msg) {
		eventMsg := &EventMsg{
			ID:        m.Header.Get("Nats-Msg-Id"),
			Subject:   m.Subject,
			EventType: m.Header.Get("Event-Type"),
			Payload:   m.Data,
			Timestamp: time.Now().UTC(),
		}
		handler(eventMsg)
	})
	if err != nil {
		return nil, fmt.Errorf("nc.Subscribe failed: %w", err)
	}
	return &natsSub{sub: sub}, nil
}

func (b *NATSBus) SubscribeChan(subject string, ch chan *EventMsg) (Subscription, error) {
	natsMsgChan := make(chan *nats.Msg, 64)
	sub, err := b.nc.ChanSubscribe(subject, natsMsgChan)
	if err != nil {
		return nil, fmt.Errorf("nc.ChanSubscribe failed: %w", err)
	}

	// Goroutine bridge from nats.Msg to EventMsg
	go func() {
		for m := range natsMsgChan {
			eventMsg := &EventMsg{
				ID:        m.Header.Get("Nats-Msg-Id"),
				Subject:   m.Subject,
				EventType: m.Header.Get("Event-Type"),
				Payload:   m.Data,
				Timestamp: time.Now().UTC(),
			}
			select {
			case ch <- eventMsg:
			default:
				// Avoid blocking bridge if receiver channel is full
			}
		}
	}()

	return &natsSub{sub: sub}, nil
}

func (b *NATSBus) JetStream() jetstream.JetStream {
	return b.js
}

func (b *NATSBus) Conn() *nats.Conn {
	return b.nc
}

func (b *NATSBus) Close() error {
	if b.nc != nil {
		b.nc.Close()
	}
	return nil
}
