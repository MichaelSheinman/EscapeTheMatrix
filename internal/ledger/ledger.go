/*
Package ledger is the record of a player's money: every movement, in order, with
the balance each one left behind.

It owns one table and holds no rule about what anything costs (§3.1). The
balance itself stays internal/character's — one column, read and written under a
lock — and a line is written here in the same transaction that moved it, so a
statement cannot disagree with the number at the top of it.

Nothing in this package writes a sentence. A line carries a [Kind] and the
authored name of the other side, and the words a player reads are composed from
the pair where they are shown. A description stored in the database would be
prose outside content/, and a player reading a two-month-old statement would be
reading whatever phrasing the game used in the month it was written.
*/
package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Kind is what a movement was.

The list is closed here and open in the schema: a new kind is a constant and a
sentence to say about it, never a migration.
*/
type Kind string

const (
	// Opening is the balance a character starts with, or the one an existing
	// character already had when the ledger was added under them.
	Opening Kind = "opening"
	// Wage is a day's structural income from a held job (§3.4).
	Wage Kind = "wage"
	// Paid is a paid action, against the day's allowance (§3.4).
	Paid Kind = "paid"
	// Kept is money a challenge run put in the player's hands.
	Kept Kind = "kept"
	// Fare is a ride across the city.
	Fare Kind = "fare"
	// Care is an instrument bought from internal/health.
	Care Kind = "care"
	// Rent is a month of the place the player lives.
	Rent Kind = "rent"
)

// Line is one movement. Amount is signed — in is positive, out is negative —
// and After is the balance it left.
type Line struct {
	ID           int64
	At           time.Time
	Kind         Kind
	Counterparty string
	Amount       simulation.Money
	After        simulation.Money
}

// Ledger is the module. The clock is injected and the transaction comes from
// the caller on every path (§3.1).
type Ledger struct {
	clk simulation.Clock
}

func New(clk simulation.Clock) *Ledger { return &Ledger{clk: clk} }

/*
Record writes one line.

Called by internal/character, from inside the statement that moved the money, so
there is no order in which a movement exists without its line. The amount is
signed by the caller because only the caller knows which way the money went, and
`after` is what the update returned rather than anything this package computed.
*/
func (l *Ledger) Record(
	ctx context.Context, tx pgx.Tx, accountID string,
	kind string, counterparty string, amount, after simulation.Money,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger (account_id, at, kind, counterparty, amount_cents, balance_cents)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		accountID, l.clk.Now(), kind, counterparty, amount.Cents(), after.Cents(),
	); err != nil {
		return fmt.Errorf("ledger: record: %w", err)
	}
	return nil
}

// Recent is the statement, newest first, at most `limit` lines long.
func (l *Ledger) Recent(ctx context.Context, tx pgx.Tx, accountID string, limit int) ([]Line, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, at, kind, counterparty, amount_cents, balance_cents
		  FROM ledger
		 WHERE account_id = $1
		 ORDER BY id DESC
		 LIMIT $2`,
		accountID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: recent: %w", err)
	}
	defer rows.Close()

	var out []Line
	for rows.Next() {
		var (
			line            Line
			kind            string
			amount, balance int64
		)
		if err := rows.Scan(&line.ID, &line.At, &kind, &line.Counterparty, &amount, &balance); err != nil {
			return nil, fmt.Errorf("ledger: recent: %w", err)
		}
		line.Kind = Kind(kind)
		line.Amount = simulation.Money(amount)
		line.After = simulation.Money(balance)
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger: recent: %w", err)
	}
	return out, nil
}
