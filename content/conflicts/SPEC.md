# Conflict spec

The game is a life simulator: a conflict is a text-message exchange with
somebody you live or work among, and it reads like one. The phone renders
`line` as their bubble, `did` as the reply you send, and `came` as their
bubble back — so each field imitates its counterpart in a real thread. Nothing
in a conflict is narrated: it is three messages, and the player works out what
happened the way anyone works it out from a text thread.

## What the player must be able to learn (the payload)

1. **`line` — what they want or did**, concrete, in the first-person words
   this kind of person would actually type. It must carry enough for the four
   answers to be weighable: what is being asked, on what footing. It is
   person-agnostic — any `{who}` of the environment sends it — so it speaks in
   the environment's register, never one person's quirk.
2. **`did` ×4 — four genuinely different moves**, each written as the reply
   you would actually send (or a plain statement of the act, where the stance
   is a deed more than a message). Not four paraphrases of one reply.
3. **`came` — what they text back**, days or hours later, in their own
   first-person words. It carries what followed by *implying* it, the way a
   real reply does: somebody who was let down says who ended up doing it and
   how long it took, and the resentment is in the wording rather than stated.
   It is the only account of how it went the player gets, so the facts have to
   be in there — but they arrive as one person telling another, never as a
   report. Same person-agnostic rule as `line`: any `{who}` could have sent it.

## What is never written

- **`{who}` in `line` or `came`.** Both are that person's own bubbles and the
  thread header names them; a bubble that speaks of its own sender in the third
  person is the register failure this SPEC exists to kill. The loader rejects
  it in either field.
- **Narration anywhere.** `came` is typed by a person on a phone, so there is
  no "word at the shop was…", no camera on the weekend, no sentence the sender
  could not have written themselves.
- **Vagueness where life is specific.** A real ask names the number and the
  reason: "could you lend me $200 till Friday? Boiler's gone." — never "a
  decent amount", "a round figure". The authored amount must be believable
  for the concept and coherent with the file's money `reading:` (a matcher
  threshold, in cents) — an ask far above the `spare` threshold reads as
  absurd against the mechanics.
  (Money still does not mechanically move in a conflict — so `came` talks
  around the money, and never states the player's balance.)
- **Future consequences in `came`** ("the next ask is larger and arrives
  sooner"). The game does not teach. A person can say "I'll remember that" —
  that is a mood, and people type it. A person cannot announce the game's
  next move.
- **Claims the player's own numbers contradict** — a balance that visibly
  never moved does not get spoken of as money handed over and returned.

## Format

```yaml
name: Asking to borrow money   # plain factual label, ≤ 50 chars
line: >-                        # 1–3 sentences, ≤ 240 chars, first person, no {who}
  ...
answers:
  - stance: yield
    did: ...                    # ≤ 80 chars
    came: >-                    # 1–2 sentences, ≤ 220 chars, first person, no {who}
      ...
```

All budgets are enforced by the loader; over budget is a boot error.

## Environment registers

Both are real people typing, neither is a performance:

- **east-side** — neighborly and informal; favours, first names, no terms
  stated because stating terms is the rude part.
- **riverfront** — cordial and transactional; the footing arrives early
  ("you'd have it back", "we can put it in writing"), the message is short
  because their time is.

## Banned (the fluff tells)

- Third-person narration inside a bubble: "…then waits, and watches which way
  you fill the silence."
- Silence/waiting/watching theatrics, meaningful pauses, negation flexes.
- Aphorism-shaped aftermath: "It is mentioned again in March."
- A reply that hands down a verdict or a moral. People are petty, wounded,
  grateful or brisk in a text; nobody summarises your character for you.
- A `came` that reports the exchange back to the player ("the move ran into
  Sunday night with the brother doing it") instead of being sent by the
  person who lived it ("ended up Sunday night, me and our Marek and a lad
  off the corner").
- Trade jargon the player has to decode: "could use a bridge" (a bridge
  loan) reads as "how would I give a bridge?". Money is asked for in plain
  words — lend, borrow, pay back.
- **Invented population members.** The population's people are authored in
  `content/environments/<key>.yaml`, and `{who}` is drawn from that roster
  at runtime — a named neighbor who isn't on it ("Joan from the first
  floor") contradicts the world, and a roster name risks colliding with the
  very person the thread is with. Refer to other locals by role ("the woman
  on the first floor", "the fella from the depot"). Named relatives of the
  asker ("our Karen", "Callum") and outside figures (a landlord, "Denny's
  garage", businesses) are fine.

## Worked example — good (calibrated 2026-08-10)

> Awkward ask — could you lend me $2,500 till the end of the month? Car
> died and I'm short till payday. You'd have it back in full, no hard
> feelings either way.

First person, a specific amount and a reason, plain words, and the reader
can weigh all four replies against it. The amount sits in the region of the
file's `spare` reading, so the prose and the matcher agree on the scale.

## Worked example — a `came` (calibrated by Michael, 2026-08-10)

The player was asked for a Saturday morning's help and said no. What followed:
the move ran into Sunday night, the brother and a lad off the corner did it,
and the player was the first person asked.

> No worries at all. Ran into Sunday night in the finish, me and my brother
> and a lad off the corner. You were the first one I asked, mind. I'll
> remember it.

Every fact is in there and not one of them is reported. The first three words
are friendly, the last five are not, and nothing anywhere says the player was
in the wrong — the reader works that out, which is the whole difference
between a thread and a lesson.

## Worked example — bad (the old register; never write this)

> {who} needs a round number for eight weeks and says so in one sentence, then
> waits, and watches which way you fill the silence.

A chat bubble from a person, describing that person from the outside: wrong
voice, wrong frame, and the "silence" is theatre a text thread cannot even
show.
