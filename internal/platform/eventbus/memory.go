package eventbus

import (
	"context"
	"strings"
	"sync"
	"time"
)

type memorySub struct {
	id      int64
	subject string
	handler func(msg *EventMsg)
	ch      chan *EventMsg
	bus     *MemoryBus
}

func (s *memorySub) Unsubscribe() error {
	s.bus.removeSub(s.id)
	return nil
}

// MemoryBus adalah implementasi thread-safe in-memory Bus untuk lingkungan pengujian
// dan fallback lokal tanpa ketergantungan broker eksternal.
type MemoryBus struct {
	mu      sync.RWMutex
	nextID  int64
	subs    map[int64]*memorySub
	closed  bool
	history []*EventMsg
}

// NewMemoryBus menginisialisasi in-memory bus baru.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		subs: make(map[int64]*memorySub),
	}
}

func (m *MemoryBus) Publish(ctx context.Context, subject, eventType, eventID string, payload []byte) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return context.Canceled
	}
	msg := &EventMsg{
		ID:        eventID,
		Subject:   subject,
		EventType: eventType,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}
	m.history = append(m.history, msg)

	// Copy active subscribers
	subscribers := make([]*memorySub, 0, len(m.subs))
	for _, sub := range m.subs {
		if matchSubject(sub.subject, subject) {
			subscribers = append(subscribers, sub)
		}
	}
	m.mu.Unlock()

	// Dispatch asynchronous
	for _, s := range subscribers {
		sub := s
		go func() {
			if sub.handler != nil {
				sub.handler(msg)
			}
			if sub.ch != nil {
				select {
				case sub.ch <- msg:
				default:
					// Drop if channel is full to prevent deadlocking publisher
				}
			}
		}()
	}
	return nil
}

func (m *MemoryBus) Subscribe(subject string, handler func(msg *EventMsg)) (Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	sub := &memorySub{
		id:      m.nextID,
		subject: subject,
		handler: handler,
		bus:     m,
	}
	m.subs[sub.id] = sub
	return sub, nil
}

func (m *MemoryBus) SubscribeChan(subject string, ch chan *EventMsg) (Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	sub := &memorySub{
		id:      m.nextID,
		subject: subject,
		ch:      ch,
		bus:     m,
	}
	m.subs[sub.id] = sub
	return sub, nil
}

func (m *MemoryBus) removeSub(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subs, id)
}

func (m *MemoryBus) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.subs = make(map[int64]*memorySub)
	return nil
}

// matchSubject memvalidasi apakah subjek cocok dengan pola wildcard NATS (* dan >).
func matchSubject(pattern, subject string) bool {
	if pattern == subject || pattern == ">" {
		return true
	}
	pTokens := strings.Split(pattern, ".")
	sTokens := strings.Split(subject, ".")
	for i := 0; i < len(pTokens); i++ {
		if pTokens[i] == ">" {
			return true
		}
		if i >= len(sTokens) {
			return false
		}
		if pTokens[i] != "*" && pTokens[i] != sTokens[i] {
			return false
		}
	}
	return len(pTokens) == len(sTokens)
}
