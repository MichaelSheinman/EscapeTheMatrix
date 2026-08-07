// Package simulation is the rules of the game, and nothing else.
//
// technical.md §3.1 calls this the most important structural rule in the
// backend: a functional core, state in and new state plus a list of effects
// out. **No database, no network, no time.Now.** Everything else in the backend
// is plumbing around it — modules load state, call in here, write what comes
// back, and act on the effects.
//
// Three things that buys, all of them already committed to elsewhere:
//
//  1. The whole simulation is testable without Postgres, in milliseconds.
//     §17 lists biomarker formulas, Karma weights and decay curves as things to
//     tune *during* implementation, and a tuning loop has to be a unit test
//     rather than a playthrough.
//  2. It is exactly the package the deferred WASM plan needs (§1). Pure from
//     the start is a build target; entangled with pgx is a rewrite.
//  3. Determinism by construction. No clock and no I/O inside means one input
//     can only produce one output, which is what makes a rollover reproducible
//     and a regression in tuning a failing assertion rather than a feeling.
//
// # The rules of this package
//
// It imports nothing outside itself — not even the rest of this repository.
// The only dependencies are `time` and `math` from the standard library, and
// `time` is used for its *types*: reading the clock is what is forbidden, and
// the interface below is how the time gets in.
//
// Nothing here mutates its arguments. Every function returns new state. The
// values are small structs of numbers, so copying is free and aliasing bugs
// across a transaction boundary are impossible by construction.
//
// Nothing here is random. There is no runtime LLM (§7) and no dice, so a given
// input can only produce one output — see TestTheSameInputProducesTheSameOutput.
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
