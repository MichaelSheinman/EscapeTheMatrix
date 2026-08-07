package realtime

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

// channel is the Postgres pub/sub channel every message is announced on.
//
// One channel for the whole application rather than one per player: a channel
// per player would mean an instance issuing a LISTEN for every connection and a
// long-lived registration to clean up after every disconnect, to save each
// instance a filter over a map it already holds.
const channel = "etm_realtime"

// How long the listener waits before reconnecting after the connection carrying
// its LISTEN dies, and how often the outbox is swept.
const (
	listenRetry = 2 * time.Second
	sweepEvery  = 5 * time.Minute
)

// Hub is the process's half of the fan-out: one Postgres listener, and the set
// of sockets this instance currently holds.
//
// It holds no message state. Everything a connection sends comes out of the
// outbox at the moment it is sent, which is what keeps the instances
// interchangeable (§3.2) — a socket that reconnects onto a different server
// resumes from exactly where it left off, because the place it left off is a
// row in Postgres rather than a buffer in a process that has since been
// replaced.
type Hub struct {
	store *store.Store
	log   *slog.Logger

	mu   sync.Mutex
	subs map[string]map[*subscriber]struct{} // account id → its sockets here

	// stopped is closed when Run returns, which is how sockets learn the
	// process is going away. Hijacked connections are invisible to
	// http.Server.Shutdown, so nothing else would tell them.
	stopped chan struct{}
	once    sync.Once
}

// subscriber is one socket's registration.
//
// wake is a doorbell, not a queue: it is buffered to one and sent to without
// blocking, so a notification storm collapses into a single "read the outbox
// again". A slow client therefore costs memory in the socket's write buffer
// and nothing here — there is no per-connection backlog to grow.
type subscriber struct {
	account string
	wake    chan struct{}
}

// NewHub returns a hub bound to its database. Run it before serving sockets.
func NewHub(st *store.Store, log *slog.Logger) *Hub {
	return &Hub{
		store:   st,
		log:     log,
		subs:    make(map[string]map[*subscriber]struct{}),
		stopped: make(chan struct{}),
	}
}

// Run holds the Postgres listener and the outbox sweep until ctx is cancelled.
//
// It returns only when told to, reconnecting on its own if the listening
// connection dies — a listener that gave up on the first network blip would
// leave every socket on this instance silently deaf, which looks exactly like
// "the game stopped happening".
func (h *Hub) Run(ctx context.Context) {
	defer h.close()

	go h.sweeping(ctx)

	for ctx.Err() == nil {
		if err := h.listening(ctx); err != nil && ctx.Err() == nil {
			h.log.Error("realtime listener", "error", err)
			select {
			case <-time.After(listenRetry):
			case <-ctx.Done():
			}
		}
	}
}

// listening holds one connection's worth of LISTEN, returning when it fails.
func (h *Hub) listening(ctx context.Context) error {
	pooled, err := h.store.Pool().Acquire(ctx)
	if err != nil {
		return err
	}
	// Hijacked out of the pool rather than borrowed: a connection with a LISTEN
	// registered on it is not interchangeable with the others, and handing it
	// back would leave a query somewhere else picking up this subscription.
	// The pool opens a replacement on demand.
	conn := pooled.Hijack()
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return err
	}

	// Whatever happened while there was no listener was missed, so every socket
	// here re-reads rather than trusting a doorbell that may not have rung.
	// This is also the normal path at startup, and it is why a lost
	// notification is a latency problem rather than a lost message.
	h.wakeAll()

	h.log.Debug("realtime listening", "channel", channel)
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return nil
			}
			return err
		}
		h.wake(n.Payload)
	}
}

// sweeping runs the outbox sweep on a timer for as long as the hub does.
func (h *Hub) sweeping(ctx context.Context) {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := h.sweep(ctx); err != nil && ctx.Err() == nil {
				h.log.Error("realtime sweep", "error", err)
			}
		}
	}
}

// subscribe registers a socket for a player's messages.
//
// Registration happens *before* the socket's first read of the outbox. The
// other order has a window — a message published between the read and the
// registration rings a doorbell nobody is holding — and the symptom would be a
// message that arrives only when the next one does.
func (h *Hub) subscribe(account string) *subscriber {
	sub := &subscriber{account: account, wake: make(chan struct{}, 1)}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[account] == nil {
		h.subs[account] = make(map[*subscriber]struct{})
	}
	h.subs[account][sub] = struct{}{}
	return sub
}

func (h *Hub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if here := h.subs[sub.account]; here != nil {
		delete(here, sub)
		if len(here) == 0 {
			delete(h.subs, sub.account)
		}
	}
}

// wake rings the doorbell for every socket this instance holds for a player.
func (h *Hub) wake(account string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs[account] {
		ring(sub)
	}
}

func (h *Hub) wakeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, here := range h.subs {
		for sub := range here {
			ring(sub)
		}
	}
}

// ring is the non-blocking send that makes the doorbell a doorbell: a socket
// that has not answered the last ring does not need a second one.
func ring(sub *subscriber) {
	select {
	case sub.wake <- struct{}{}:
	default:
	}
}

// close tells every socket on this instance to go.
func (h *Hub) close() { h.once.Do(func() { close(h.stopped) }) }
