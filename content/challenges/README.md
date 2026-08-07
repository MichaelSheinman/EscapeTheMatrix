# Challenges

One directory per family, one directory beneath it per variant — and those two
directory names *are* the challenge's identity (technical.md §5.3). Nothing
inside the files repeats them, so a rename is a `git mv` rather than an edit in
two places that can half-happen.

```
content/challenges/resolve_conflict/a/
  challenge.yaml     # mechanic type, availability, the event that opens it
  dialogue.yaml      # whatever the mechanic type reads — its schema, its file
```

## The line

**Mechanic types are code. Challenges are data** (§5.2).

- A **mechanic type** is implemented once, in Go, with a typed config schema:
  `internal/challenge/dialogue.go` is one.
- A **challenge** is `mechanic type + config document + content files`, and
  lives entirely under this directory.
- **New content in an existing type is data only. A new mechanic is code.**

So: *writing another `resolve_conflict` variant, or another challenge of any
shape a mechanic type already runs, is two YAML files and no Go.* If you find
yourself opening `internal/` to add a challenge, one of two things is true —
either you are adding a mechanic type, which is code and is meant to be, or the
type you are using is missing something and it should be added to that type for
every challenge on it rather than to yours.

Going the other way, **the discipline is to extract a type on the *second*
instance, not the first** (§5.2). At V1's roughly 2:1 type-to-instance ratio the
config machinery does not pay for itself, so the first challenge of a new shape
is written as Go — `internal/challenge/stoop.go` was, for a while — and the type
is lifted out of it once there is a second challenge to be the type *of*. A type
with one instance is a rename, not an abstraction.

`challenge.yaml` is the one file every challenge has, whatever runs it:

```yaml
name: The argument on the stoop
mechanic: dialogue_branch      # a registered mechanic type — code, not content
opens: two-people-arguing      # optional: the awaiting event this is behind
where:                         # optional: where its own entry point stands
  places: [east-side]          # or districts: / layers:
  zones: [home-door]
```

A challenge names `opens`, `where`, or both. Naming neither is a failed build:
technical.md §4 keeps availability and offer as separate concepts, and a
definition with neither is content nobody can reach.

Everything else belongs to the mechanic type, which reads its own documents out
of this directory and validates them against its own schema. A file the type
does not read is also a failed build — a config document nothing reads looks
authored and is not.

## `dialogue_branch`

The one type so far, read by `internal/challenge/dialogue.go`, authored in
`dialogue.yaml`. People in a place, saying things, and a question the player
answers by committing to one of several readings — each *received* rather than
understood, and each leading somewhere different.

```yaml
opening: the-steps            # optional; the first scene otherwise
scenes:
  - key: the-steps            # lower case, digits, - or _
    where: The steps of your building, a quarter to seven
    beats:
      - line: They see you at the same moment and both stop.
      - who: Marisol          # absent for the narration itself
        line: You went there on your own.
    ask: There is a way of coming into somebody else's argument.
    options:
      - key: sit
        text: Sit down on the step below them.
        taken: She moves her bag off the step to make room for you.
        then: laying-it-out   # absent ends the run
        weight: 2             # never shown; negative allowed
        moves:                # authored hidden factor deltas
          kindness: 30
outcomes:
  - key: talked-past
    at-least: -2              # a floor, not a range
    reaction: >-
      It starts again below you before you have your own door open.
```

Every field above is required except `opening`, `then`, `weight`, `moves` and a
beat's `who`. Decoding is strict, so a misspelt field is a failed build rather
than a line of content silently ignored.

- **`taken` is not optional and is the point.** §3.10: "the characters then react
  according to how they *perceive* your interpretation." It hangs off the option
  rather than off the scene it leads to, because two readings can arrive in the
  same room and land completely differently.
- **`weight` is hidden scoring** (§3.10: "never surfaced as a grade, never
  confirmed as right"). It never crosses the wire. What the player is told is the
  `reaction` of the band their total landed in, and nothing else.
- **`moves` names the three factors scored from committed choices** —
  `intelligence`, `stability`, `kindness`. Health and money are read from the
  world rather than granted (§3.5) and are refused here. Magnitudes are against
  `simulation.CreditHalf`, which is 240: a whole passage through a challenge pays
  somewhere between about 60 and 200 across two or three factors.
- **The graph is checked by the framework, not by the type**: no cycles, no
  unreachable scene, no scene that offers nothing, at least one scene where
  something is actually chosen, and outcome floors that the plan's own worst and
  best passage can both land inside. An outcome nobody can reach is reaction text
  nobody will read, and that fails the build.

## What ships

- **`resolve_conflict/a` — the argument on the stoop.** §3.10's own worked
  example: Marisol and Dev on the step, eighteen thousand to sign away a lease,
  nine of the twelve already signed. Written as Go for #24 and moved into
  `dialogue.yaml` unchanged for #25. It names both ways in.
- **`resolve_conflict/b` — the Sunday at the laundromat.** Noor and Iman, their
  father, and a room on Cardwell Road if the deposit is in by Thursday. Authored
  as these two files with no Go written for it, which is the only thing that
  proves the line above is in the right place. It names only `where`.
