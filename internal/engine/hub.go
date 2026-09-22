package engine

import "sync"

// Event is pushed to desk clients over SSE. Doctype/DocName, when set, name
// the document the event is about so the hub can withhold it from
// subscribers who may not read that document (B20).
type Event struct {
	Name    string `json:"event"`
	Payload any    `json:"payload"`
	User    string `json:"-"` // "" = broadcast
	Doctype string `json:"-"`
	DocID   string `json:"-"`
}

// Authorizer answers whether a subscriber may see events about a document.
type Authorizer func(doctype, name string) bool

type subscriber struct {
	user      string
	authorize Authorizer
}

// Hub fans events out to subscribed connections.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]subscriber
}

func NewHub() *Hub { return &Hub{subs: map[chan Event]subscriber{}} }

// Subscribe registers a connection. A nil authorize means the subscriber
// only receives events that name no document.
func (h *Hub) Subscribe(user string, authorize Authorizer) chan Event {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = subscriber{user: user, authorize: authorize}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *Hub) Publish(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, sub := range h.subs {
		if ev.User != "" && ev.User != sub.user {
			continue
		}
		if ev.Doctype != "" && (sub.authorize == nil || !sub.authorize(ev.Doctype, ev.DocID)) {
			continue
		}
		select {
		case ch <- ev:
		default:
		}
	}
}
