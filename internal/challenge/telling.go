package challenge

/*
What a challenge says, and who is allowed to say it.

A [Plan] is a graph of keys. It is everything the framework needs to save a run,
resume it, score it and pay it — and it is nothing a player could read. The
words are the mechanic's: §5.2 gives a mechanic type "a Go handler plus a
matching client scene module, with a typed config schema", which is to say that
what a scene *says* is authored inside the mechanic and rendered by the module
built for it.

That leaves one gap, and this file is it. The transport cannot reach a mechanic
— internal/api has never heard of one, and §3.1 means it never will — and a
mechanic cannot reach the wire. So the framework carries the words between them,
and the whole of its involvement is carrying:

  - it does not compose them,
  - it does not validate them (a mechanic's words are its own schema's business,
    exactly as its config is),
  - it does not store them: nothing in this file is ever written to a table, and
    a run resumed after a deploy is narrated by the content in the new binary
    rather than by a copy of yesterday's writing.

# Why there are no numbers here

[Told] carries text and keys and nothing else, and that is the design rather
than an omission. §3.10: hidden scoring is "never surfaced as a grade, never
confirmed as right", and §5.1 puts the player's whole account of how it went in
the reaction text of the bucket their trail landed in. A field on this struct
holding an option's weight, a running total, or the name of the band they are
currently in would be the grade, arriving by the one route left open.

The one thing a mechanic *may* say about position is how far through the run is,
which is what [Plan.Ahead] answers — a length, not a judgement, and the same
thing the six-minute promise in §3.10 is made of.
*/

// Beat is one line of a scene: somebody, and what they said.
//
// Who is empty for the narration itself — the sentences describing the room
// rather than anybody in it — which is a distinction the screen draws and this
// package does not care about beyond keeping the two apart.
type Beat struct {
	Who  string
	Line string
}

// Choice is one of the scene's authored options, in the words a player reads.
//
// The key is the plan's, because it is what [Challenges.Choose] takes back. The
// text is the mechanic's. Nothing else is here: an option's weight and its
// factor deltas are in the plan where the framework can pay them, and a copy of
// either in the thing sent to a screen would be the hidden scoring, shown.
type Choice struct {
	Key  string
	Text string
}

/*
Told is one moment of a run, as the mechanic tells it.

Everything a screen needs to put the player back exactly where they were, which
is §5.1's save-and-resume seen from the front: a returning player is handed the
same words they left, because the scene is the unit of both.

Taken is the part worth naming. §3.10 asks that "the characters then react
according to how they *perceive* your interpretation" — so what a run carries
forward is not only which branch it took but how the last commitment landed on
the people in the room, and that sentence is authored per option rather than per
scene. It is empty at the opening, where nothing has been committed yet.
*/
type Told struct {
	// Where the run is standing, in the content's own words.
	Where string
	// Taken is how the last thing the player committed to was received —
	// which is not the same as what they meant by it.
	Taken string
	// Beats are what is said at this scene, in order.
	Beats []Beat
	// Ask is the question the options answer, in the second person.
	Ask string
	// Choices are the authored positions, one per option at this scene. None of
	// them is correct (§3.10) and none of them is marked.
	Choices []Choice
	// Closing is the reaction authored for the outcome bucket a finished run
	// landed in (§5.1), and is empty until it has finished. It is the only
	// account of how it went that the player ever gets.
	Closing string
	// Stage is how many scenes have been stood in, counting this one, and
	// Stages is the most there can be on the way out from here. Progress rather
	// than score: it is what lets a screen say "about six minutes" and mean it.
	Stage  int
	Stages int
}

/*
Narrator is a configured challenge that can say what is happening in its own
scenes.

Optional on [Configured] rather than required of it, because the two answer
different questions and a challenge that cannot yet speak is still one the
framework can validate, save and score. A type that never implements this is a
challenge nobody can be shown — which is a thing worth being able to build
halfway to, and a thing [Say] reports honestly rather than papering over.

It hangs off the *configured challenge* rather than off the [Mechanic] because
the words belong to one challenge and the mechanic is shared by all of them: a
type that narrated from the registry would have to keep a map of code to script
and find its own way back into it, and would get that wrong for exactly as long
as a hot reload takes.
*/
type Narrator interface {
	// Say is what the player is looking at, given where the run is standing and
	// everything it has committed. It reads the run and changes nothing.
	Say(run Run) (Told, bool)
}

/*
Say asks a run's own challenge what is happening.

The false is a real answer with two causes, and neither is an error: the
challenge was withdrawn from content while somebody was part way through it — so
the definition on the run carries nothing a mechanic made — or its type has no
words yet. Both leave a caller with a run it can describe as a fact — begun,
standing at a scene — and nothing to show, which is better than a sentence this
package invented on a mechanic's behalf.
*/
func Say(run Run) (Told, bool) {
	teller, speaks := run.Challenge.Made.(Narrator)
	if !speaks {
		return Told{}, false
	}
	return teller.Say(run)
}

/*
Ahead is the most scenes a run can still stand in, counting the one named.

The longest path rather than the shortest, so a progress indicator never
overtakes itself: a run told it was at four of four and then offered a fifth
scene has lied to the player about the one thing this number is for. The graph
is already known to be acyclic ([Plan.acyclic]), so the walk terminates.

A scene the plan does not have is zero, which is the same tolerance every other
read of withdrawn content in this package has.
*/
func (p Plan) Ahead(key string) int {
	known := map[string]int{}

	var walk func(string) int
	walk = func(at string) int {
		if got, seen := known[at]; seen {
			return got
		}
		scene, ok := p.Scene(at)
		if !ok {
			return 0
		}
		most := 0
		for _, o := range scene.Options {
			if o.Ends() {
				continue
			}
			if onward := walk(o.Then); onward > most {
				most = onward
			}
		}
		known[at] = most + 1
		return most + 1
	}
	return walk(key)
}
