// Package simulation is the rules of the game, and nothing else.
//
// A functional core: state in, new state plus a list of effects out, with no
// database, no network and no time.Now (§3.1). Everything else in the backend is
// plumbing around it — modules load state, call in here, write what comes back,
// and act on the effects.
//
// That buys three things. The whole simulation is testable without Postgres, in
// milliseconds, which is what a tuning loop over biomarker formulas, Karma
// weights and decay curves needs (§17). It is the package the deferred WASM plan
// needs (§1). And it is deterministic by construction: no clock and no I/O means
// one input can only produce one output.
//
// It imports nothing outside itself, not even the rest of this repository. The
// only dependencies are `time` and `math`, and `time` is used for its types —
// reading the clock is what is forbidden, and the interface below is how the
// time gets in.
//
// Nothing here mutates its arguments. Every function returns new state, and the
// values are small structs of numbers, so copying is free.
//
// Nothing here is random. There is no runtime LLM (§7) and no dice — see
// TestTheSameInputProducesTheSameOutput.
package simulation

import "time"

// Clock is how the time gets in.
//
// Declared here rather than imported, because this package imports nothing
// (§3.1). Any type with this method satisfies it — internal/clock.System in the
// server, internal/clock.Fixed in a test — and neither of them is named here.
type Clock interface {
	Now() time.Time
}

// Effect is something that happened which the impure world has to deal with.
//
// This is the other half of "state in, new state plus a list of effects out".
// The core cannot write a row, publish a message, or schedule a job — so when a
// rule notices something worth acting on, it says so, and the module that
// called it decides what that means. A threshold crossed today is a socket
// message now and an event release later (§4) without this package learning
// that either exists.
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
