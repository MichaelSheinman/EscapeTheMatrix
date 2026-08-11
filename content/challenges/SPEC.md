# Challenge spec — interviews (interim)

Challenges are moving to richer first-person UI pieces; This is already the case
for the house_fire challenge, but still WIP for interviews. Until that lands, the
authored text is the minimum a challenge needs to run, and nothing more. This
file currently governs **interviews**; the general challenge spec gets written
when the first-person UI decision is made.

## An interview is a real interview, minimal

The player is in a room answering an interviewer. No scene-setting prose, no
atmosphere — the banned register ("The fourth floor is carpet…") is exactly
what this replaced. Three linear scenes: an opening question, a scenario, a
close.

Per scene:

- `where` — one plain line, ≤ 60 chars ("The transport office, Tuesday
  morning.").
- One beat: the interviewer's actual spoken question, ≤ 180 chars, labelled
  with a role (`MANAGER`, `DIRECTOR`), never a name.
- `ask` — short and functional ("What do you say?").
- **Four options**: a weak, a mid, and a strong real answer (≤ 90 chars,
  written with a person's voice — a weak answer is a *real slightly-off
  answer*, not a placeholder), plus **one troll option** — an obviously
  wrong, genuinely funny thing a player might say to watch it blow up.
  Deadpan-plausible, no fourth-wall, no memes; the `taken` (≤ 120 chars, the
  interviewer's visible response) is where the joke lands. Nothing marks
  which option is which (§3.2); option keys are semantic, never graded —
  keys cross the wire.

## Weights and outcomes

Weights: weak 0, mid 2, strong 3, troll −4. Four outcomes per interview —
reject / kept-on-file / offer / strong-offer — with floors −12 / 4 / 7 / 9,
so one troll answer guarantees rejection: clicking the funny option costs the
job, which is the price of seeing it. Outcome keys are referenced by
`content/jobs/*` `offered:` lists and never change casually. Reactions are
how you actually hear a result — a call, an email, a letter — 1–2 sentences,
≤ 220 chars, no morals.

Interviews shared by two jobs (`interview/b`: operations + billing;
`interview/c`: clinic + bank counter) stay job-generic in every string.

## Never written

- Scene-setting narration, atmosphere, objects with history.
- Named characters — the interviewer is a role.
- Factor `moves` — the minimal interview moves nothing; the outcome is the
  consequence.
- Anything the deletion test kills.
