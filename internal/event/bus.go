package event

import "sync"

// Bus is a tiny in-process pub/sub (optional decoupling for future plugins).
type Bus struct {
	mu   sync.RWMutex
	subs map[string][]func(any)
}

func NewBus() *Bus {
	return &Bus{subs: make(map[string][]func(any))}
}

func (b *Bus) Subscribe(topic string, fn func(any)) {
	if fn == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[topic] = append(b.subs[topic], fn)
}

func (b *Bus) Publish(topic string, payload any) {
	b.mu.RLock()
	var list []func(any)
	list = append(list, b.subs[topic]...)
	b.mu.RUnlock()
	for _, fn := range list {
		fn(payload)
	}
}
