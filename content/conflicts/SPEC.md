# Conflict spec

A conflict is a text-message exchange with an NPC within the current environment. The phone renders `line` as the text the NPC sent, `did` as a reply option for the main character, and `came` as the response the NPC sent back.

THERE IS NO NARRATION - CONFLICTS READ LIKE REAL, SIMPLE, HUMAN CONVERSATIONS. 

1. **`line` — what an NPC wants/did**, First-person words. 
2. **`did` a list of 2-4 genuinely different moves**, each written as the reply
   you would actually send (or a plain statement of the act, where the stance
   is a deed more than a message). Not four paraphrases of one reply.
3. **`came` — what they text back**, NPC's response in their own
   first-person words. Depends on character - some NPCs will give responses that are upset, others will be happy, others will explain what happenned.   

## What is never written

- **`{who}` in `line` or `came`.** Both are that person's own bubbles and the
  thread header names them; a bubble that speaks of its own sender in the third
  person is the failure this SPEC exists to kill. The loader rejects
  it in either field.
- **Narration anywhere.** `came` is typed by a person on a phone.
- **Vagueness where life is specific.** A real ask names the number and the
  reason: "could you lend me $200 till Friday? Boiler's gone." — never "a
  decent amount", "a round figure". The authored amount must be believable
  for the concept and coherent with the file's money `reading:` (a matcher
  threshold, in cents) — an ask far above the `spare` threshold reads as
  absurd against the mechanics. (Money still does not mechanically move in a conflict — so `came` talks
  around the money, and never states the player's balance.)
- **A pronoun with nobody behind it.** A `came` may name a third party — a
  child, a landlord, the boiler man, whoever else was there — but the player
  has to have been told they exist, and the only thing the player has read is
  the `line`. So a "he" or a "she" in a reply must point at somebody the
  `line` already put on the page, or be named outright. `the-tenner` shipped
  three replies about "he" — the man serving at the till — off a line that
  mentioned a card, the bread and the mince and no person at all, and the
  reply read as a sentence from a different story (2026-08-16).
- **Future consequences in `came`** ("the next ask is larger and arrives
  sooner"). The game does not teach. A person can say "I'll remember that" —
  that is a mood, and people type it. A person cannot announce the game's
  next move.
- **Claims the player's own numbers contradict** — a balance that visibly
  never moved does not get spoken of as money handed over and returned.

## Format

```yaml
with: [tomasz]                  # who sends it — required, never omitted
after:                          # what must already have passed with them
  - the-quick-hand              #   any answer will do
  - { conflict: the-tenner, stance: yield }   # or a particular one
every: 7                        # days before it comes round; omit to say it once
name: Asking to borrow money    # plain factual label, ≤ 50 chars
line: >-                        # 1–3 sentences, ≤ 240 chars, first person, no {who}
  ...
answers:
  - stance: yield
    did: ...                    # ≤ 80 chars
    came: >-                    # 1–2 sentences, ≤ 220 chars, first person, no {who}
      ...
    title: ...                  # ≤ 64 chars, second person, {who} required
```

All budgets are enforced by the loader; over budget is a boot error.

## `title:` — the answer as a line in the record

One line naming **what your character did**, in the second person, with `{who}`
in it: `You did not cover {who}'s $12`. It is what an escalation about this
answer is called, so it is read cold — weeks later, in a list, beside a date,
next to fifteen other rows. Assume no other context is on screen.

The rule that decides every one of these: **write the doing, never the landing.**

- `You did not cover {who}'s $12` — a title.
- `You turned down the van on Saturday` — a title.
- `{who} is upset with you` — not a title. It is the conclusion the player is
  supposed to reach by reading the moment, handed to them before they read it.
- `It went badly`, `A costly refusal`, `You should have helped` — not titles.

The `came:` field already says how it landed, in their words, and the counts on
the playbook's log are where a pattern becomes visible. A title that graded the
moment would do the player's thinking for them (technical.md §3.2) and would do
it in the one place they cannot avoid reading.

Second person and past tense, because it is their own record. No question marks,
no full stop, no adverbs of judgement (*badly*, *rudely*, *fairly*). `{who}` is
required and is the only substitution — unlike `came:`, where the person's own
name is banned because they are the one typing. A title is written *about* them.

Same conflict, four answers, four titles: they must be tellable apart at a
glance, because a player who answered the same conflict four different ways over
a month sees all four rows together.

## Who sends it, and when

`with:` is required. There is no "anybody" default, because a default is what
let a rota and a boiler man be sent by the man behind the till — a pathway that
genuinely fits the whole population names the whole population, which costs six
words and means somebody read all six and agreed.

`after:` is what must already have passed **with that same person**. A moment
that remembers something ("I know I still owe you for the allotment afternoon")
must name what it remembers, or it will arrive from a stranger. Naming a stance
as well is how a thread branches on how somebody was treated. The loader refuses
a requirement that names a pathway the population has not got, one that shares
no sender with the file declaring it, or a loop.

`every:` is a cadence in days. Without it a pathway is **said once and spent**,
which is what a thread with an ending needs — and it is why every person also
wants two or three standing habits that do come round, or they run out of things
to say and go quiet for good. Every person must have at least one `easy` pathway
with no `after:`, or nothing can ever open a conversation with them; the loader
checks that too.

## Environment register

Real people typing, not a performance:

- **wilmington** — neighborly and informal; favours, first names, no terms
  stated because stating terms is the rude part.

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
