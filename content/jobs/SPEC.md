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

- Pay, in any form. The file states `pay` and the UI renders it; the prose never
  mentions money.
- Hours, shift times, or day counts. Rendered from `hours` and `shift`.
- Anything the mechanical fields contradict: a job whose `shift` starts at 21:00
  is overnight work, and the title and text must read as overnight work.
- The experience routes, in the prose. The `experience` block is rendered as its
  own section; a qualifications line restating it is the same fact twice.

## The mechanical fields

There is no `tier` (2026-08-15). Jobs used to sit on a five-rung ladder that
owned the pay, the hours, the shift's nights and the capability floor, which
meant two jobs sharing a number were the same job in every way a player could
measure. §3.15 says progression is lateral — the player widens rather than
climbs — so every job now states its own terms.

| field | what it is |
| ----- | ---------- |
| `pay` | cents a day. Structural income (§3.4): it arrives because the job is held and the day happened. |
| `hours` | the length of the shift, a whole number, 1–16. |
| `shift` | when it starts, `"HH:MM"`. At or after 18:00 (or before 05:00) is night work, derived — never authored twice. Otherwise 05:00–14:00. |
| `fed` | whether the shift leaves the player's eating alone. |
| `requires` | the capability floor by factor name, 0–100. Absent takes anybody. |
| `experience` | the previous work that qualifies somebody — see below. |
| `delegated` | whether the job runs while the player is elsewhere and hands them situations (§3.14's loop). A delegated job must have situations authored for it or the build fails. |

## Experience

The routes of previous work a posting asks for, and what replaced the tier as
the thing that draws a career path. Exactly one of two keys, and the operator is
always written out rather than inferred from the shape:

```yaml
experience:
  all_of:                 # one route: every job in it, for its days
    bank-counter: 45

experience:
  any_of:                 # several routes; any one of them qualifies
    - all_of:
        dispatch: 45
    - all_of:
        dispatch: 20
        warehouse-nights: 60
```

Days are **cumulative and never lost**: two stints of twenty days are forty days
of experience, and leaving a job costs the wage and nothing else. Depth stops at
two — `any_of` holds `all_of` blocks and nothing else.

Absent is an entry-level posting. Every environment needs at least one, or
nobody living there can start working; the loader refuses to boot otherwise, and
also refuses a route naming a job in another environment or a cycle of jobs each
requiring the other.

Experience gates the *interview*, not the hire. A player who has not finished a
route sees no button; one who sits an interview and passes it is taken on.

More than one job may ask for the same experience; that is what makes the board
a choice rather than a queue. What separates two jobs is the work, the wage, the
toll, the hours, and where they are hired.

## Format

```yaml
name: Overnight laundromat attendant   # a realistic job title, ≤ 6 words
shift: "21:00"                          # start; at or after 18:00 is night work
pay: 8200                               # cents a day
hours: 8                                # length of the shift
fed: false                              # omit when false
requires:                               # omit entirely for an entry-level job
  stability: 12
experience:                             # omit entirely for an entry-level job
  all_of:
    warehouse-nights: 30

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
- Prestige prose on the senior jobs. The old voice re-enters with altitude: the
  "nobody-but-you" cadence ("decisions no one can check for you", "whenever
  anything goes wrong") and rhetorical tricolons. A principal's posting is still
  an ad, not a meditation on responsibility.
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
the job's own `hours` and `shift`.
