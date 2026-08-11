# Job posting spec

The game is a life simulator: a job posting reads like a real job posting,
trimmed to game length. The player draws their own conclusions from realistic
material. There is no literary voice anywhere in a posting.

## What the player must be able to learn (the payload)

1. **What the work is** — concrete duties, the way an employer would list them.
2. **What it asks of a person** — qualifications, plainly stated.
3. **The toll, unmarked** (§3.2) — the role's hidden factor drift must be
   inferable from the duties/qualifications by a careful reader, and no line is
   ever flagged as the important one. A kindness −6 job reads as work that
   hardens you (pushing people, enforcing on people); a kindness +4 job reads
   as care work; a stability toll reads as pace, pressure, or isolation. Never
   name a factor. Every posting has the same sections, so structure carries no
   signal about which line matters.

## What is never written (mechanics own these — the UI injects them)

- Pay, in any form. Rendered from the rung + premium.
- Hours, shift times, or day counts. Rendered from the rung and `shift`.
- Anything the simulation contradicts: a tier-1 posting is an overnight job
  because tier 1 is `Nights: true`; the title and text must agree with that.

## Tier

`tier` is which rung of §3.14's ladder the role sits on, 1 to 5. It is the only
mechanical field in the file, and it settles more about the posting than the
prose does: the rung owns the pay, the hours, whether the shift is nights, and
the capability a player is hired against. A file cannot state any of those, which
is why it cannot drift from them.

| tier | what the rung is | the post | hired against |
| ---- | ---------------- | -------- | ------------- |
| 1 | the bottom, and it takes anybody | 10 hours, nights | nothing |
| 2 | the first step up | 9 hours, days | a body that has held together, and a little of both hidden factors |
| 3 | the first active tier: it runs while the player is elsewhere and hands them situations | 10 hours, days, fed | the above, higher, plus money |
| 4 | senior | 11 hours, days, fed | the above, higher, plus kindness |
| 5 | where the ladder stops naming jobs and starts naming lives — politician, founder | 12 hours, days, fed | most of the board at once |

`simulation.Rungs()` in `internal/simulation/work.go` is the authority for every
number; the table is here so that a tier can be chosen without reading Go.

Choosing a tier commits the posting to three things:

- **The shift.** Tier 1 is a nights rung, so its `shift` must start at or after
  18:00 and the title and duties must read as overnight work. Tiers 2–5 are day
  rungs and start between 05:00 and 14:00. The loader rejects a `shift` that
  disagrees with the rung.
- **The qualifications.** They should read as what that rung's requirement asks
  of a person — a tier-1 posting can honestly say "no experience required", and a
  tier-4 one cannot. Never name a factor.
- **Nothing about the pay.** Two roles on the same rung earn the same before
  `premium`, and a posting that hints at money is over the line in "What is
  never written".

More than one role may sit on a rung; that is what makes the board a choice
rather than a queue. What separates two roles on the same tier is the work, the
toll, and where they are hired.

## Format

```yaml
tier: 1                                 # which rung, 1–5; see "Tier" above
name: Overnight laundromat attendant   # a realistic job title, ≤ 6 words
shift: "21:00"                          # start time; end is derived from the rung's hours

posting:
  intro: >-                             # 1–2 sentences, employer voice, ≤ 220 chars
    ...
  duties:                               # 3–5 bullets, verb-first, ≤ 70 chars each
    - ...
  qualifications:                       # 2–4 bullets, ≤ 70 chars each
    - ...
```

All budgets are enforced by the loader; a posting over budget is a boot error.

## Banned (the fluff tells)

- The literary register in any form: atmosphere, mood-setting, oblique
  significance ("Nobody asks you anything", "paid Friday, to the account").
- Named characters, backstory, gossip, or objects with history ("Auntie Pat is
  in on Tuesdays", "three machines have been on the list since spring").
- Sentences that survive the deletion test: delete it — if the reader lost no
  information about the work or its demands, it was fluff.
- Odd clarifications and negation flexes ("not the machines", "has never once").
- Anything a real employer would not put in a posting — including admitting its
  own chaos ("deliveries that arrive without notice", "when the plan falls
  apart"; a real ad says "schedules change").
- Prestige prose at senior tiers. The old voice re-enters with altitude: the
  "nobody-but-you" cadence ("decisions no one can check for you", "whenever
  anything goes wrong") and rhetorical tricolons. A tier-5 posting is still an
  ad, not a meditation on responsibility.
- Repeating one house verb for a toll across the catalogue (chase/press for
  kindness−). Vary the carrier lines file to file.

## Worked example — good

```yaml
name: Overnight laundromat attendant
posting:
  intro: >-
    Eastside Wash & Fold is looking for a reliable attendant to run the
    counter and machines overnight.
  duties:
    - Take in wash-and-fold orders; run washers and dryers
    - Fold, bag, and return completed orders
    - Handle the till; keep the counter area clean
  qualifications:
    - Dependable; able to stand a full shift
    - Comfortable with a regular customer base
    - No experience required
```

The kindness toll is present ("regular customer base") and unmarked.

## Worked example — bad (the old register; never write this)

> Ten hours of machines, change and other people's sheets, four doors from
> where you live. [...] Auntie Pat is in on Tuesdays and will tell you about
> her sister. The owner is in on Fridays with the float and has never once
> counted it in front of anybody.

Every sentence fails: no duties an employer would list, named characters,
objects with history, negation flex, and the schedule it implies contradicts
the rung's mechanics.
