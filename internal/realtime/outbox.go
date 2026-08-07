package realtime

import (
	"context"
	"encoding/json"
	"fmt"
)

// batch bounds one read of the outbox. A client that was away for a while comes
// back to a drain of several batches rather than one unbounded query, so a long
// absence cannot turn into a single enormous allocation.
const batch = 200

// message is one outbox row on its way to a socket.
type message struct {
	seq     int64
	kind    string
	payload json.RawMessage
	// origin is the session whose command caused this, empty when the world
	// acted on its own. The socket compares it against its own session id.
	origin string
}

// after reads the player's messages past a sequence number, in order.
//
// This is the *only* read of the outbox: a connection catching up after a drop
// and a connection being told about something that happened a moment ago run
// exactly the same query. Rows that a socket will discard as its own echo are
// returned too, so that the caller advances its cursor past them rather than
// re-reading them on every wake.
func (h *Hub) after(ctx context.Context, account string, seq int64) ([]message, error) {
	rows, err := h.store.Pool().Query(ctx, `
		SELECT seq, type, payload, coalesce(origin, '')
		FROM realtime_outbox
		WHERE account_id = $1 AND seq > $2
		ORDER BY seq
		LIMIT $3
	`, account, seq, batch)
	if err != nil {
		return nil, fmt.Errorf("realtime: read outbox: %w", err)
	}
	defer rows.Close()

	var out []message
	for rows.Next() {
		var m message
		if err := rows.Scan(&m.seq, &m.kind, &m.payload, &m.origin); err != nil {
			return nil, fmt.Errorf("realtime: read outbox: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// resume decides where a connecting client is put in the stream, and whether
// anything it needed is already gone.
//
// head is the player's current last sequence number. A client that is not
// resuming starts there and is told about nothing that happened before it
// arrived — the page it just loaded already reflects that. A client that *is*
// resuming keeps its own place, unless what it asked for has been swept, in
// which case there is no honest way to continue: the patches still in the
// outbox would be applied on top of a state missing the ones that are not, so
// it is moved to head and told to rehydrate instead.
func (h *Hub) resume(ctx context.Context, c Client) (cursor int64, w Welcome, err error) {
	var head int64
	var oldest *int64
	if err := h.store.Pool().QueryRow(ctx, `
		SELECT coalesce((SELECT last_seq FROM realtime_stream WHERE account_id = $1), 0),
		       (SELECT min(seq) FROM realtime_outbox WHERE account_id = $1)
	`, c.Account).Scan(&head, &oldest); err != nil {
		return 0, Welcome{}, fmt.Errorf("realtime: read stream head: %w", err)
	}

	w = Welcome{Session: c.Session, Since: c.Since, Head: head}
	switch {
	case !c.Resume:
		return head, w, nil
	case c.Since >= head:
		// Caught up, or claiming to be ahead of a stream that cannot have gone
		// backwards for any reason a client should trust — a restored database,
		// a doctored URL. Both end at head; the second is a gap.
		w.Gap = c.Since > head
		return head, w, nil
	case oldest == nil || *oldest > c.Since+1:
		// The next message this client needs is no longer retained.
		w.Gap = true
		return head, w, nil
	default:
		return c.Since, w, nil
	}
}

// sweep deletes messages older than Retention.
//
// Running it on every instance is deliberate: the work is one indexed DELETE,
// duplicating it costs a no-op, and making it the job of a single elected
// instance would be a coordination problem invented to save nothing. It moves
// to the job queue when that arrives (§4).
func (h *Hub) sweep(ctx context.Context) error {
	tag, err := h.store.Pool().Exec(ctx, `
		DELETE FROM realtime_outbox WHERE created_at < now() - make_interval(secs => $1)
	`, Retention.Seconds())
	if err != nil {
		return fmt.Errorf("realtime: sweep outbox: %w", err)
	}
	if n := tag.RowsAffected(); n > 0 {
		h.log.Debug("realtime outbox swept", "messages", n)
	}
	return nil
}
