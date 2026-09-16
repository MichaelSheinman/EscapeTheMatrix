// Package simulation is the rules of the game: state in, new state plus a list
// of effects out, with no database, no network and no time.Now. It imports
// nothing outside itself, mutates none of its arguments, and is deterministic.
package simulation

import "time"

// Clock is how the time gets in.
type Clock interface {
	Now() time.Time
}

// Effect is something that happened which the impure world has to deal with.
//
// Effects are values, not commands: they describe what happened, never what to
// do about it. "Recovery crossed below poor" rather than "send a notification".
type Effect interface {
	// effect keeps the set closed to this package, so a type switch over it
	// in a caller is a switch over a list that cannot grow behind its back.
	effect()
}

// Effects is the list a rule returns. Nil when nothing happened, which is the
// ordinary case and costs nothing.
type Effects []Effect

// with appends without disturbing the nil-is-empty convention.
func (e Effects) with(fx ...Effect) Effects { return append(e, fx...) }
