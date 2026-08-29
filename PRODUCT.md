# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Players of a long-running life simulator, playing in a desktop browser alongside
their real life rather than in binge sessions. One real calendar day is one game
day, so the product is used in short returns over months.

**Not yet confirmed by the user (inferred from the repository, pending
confirmation):** the specific target audience and acquisition context. The
design documents state "user growth first, monetization deferred entirely" but
do not name a demographic.

Target Audience: We are targetting the psychographics of users who are interested in 
personal growth, have a field where they want to grow within, believe there are things
in life to explore beyond what they are aware of. 

## Product Purpose

A life simulator in which the player builds a life — career, health, habits,
money, relationships, travel. The systems that make the character thrive are the
same ones that work in reality, so the simulation's reward function is the
curriculum: if sleep genuinely improves biomarkers, and biomarkers genuinely
improve job-interview outcomes, the player learns why sleep matters by
optimising rather than by being told.

Success is a player who keeps returning over months and who, through play,
discovers that a rule set which worked in one environment fails in another.

## Positioning

The thesis the game exists to demonstrate: **different situations call for
different appropriate actions, and there is no universal correct rule set.** The
game proves this mechanically — the player masters one environment, hits its
ceiling, travels, and finds their hard-won rules fail somewhere else.

It is a game, not a growth platform. Frameworks are expressed as characters and
situations, never named and never explained in-game. The message is never
delivered; it is what the mechanics do.

## Operating Context

- One game day equals one real calendar day, on the player's own clock. Time of
  day is real: night runs from 8 PM to 6 AM and the world plate changes with it.
- Actions resolve instantly. No build timers, no energy refills, no waiting.
- The world is a rendered isometric city plate per location, with the character
  drawn on it. Screens open over that world.
- Movement between places is walking, and walking is where events happen.
- Screens currently reachable: the world, where you stand, your playbook, your
  body, the job board, your life, routines, the phone, events, challenges.

## Capabilities and Constraints

**Terminology that is product truth, not decoration**
- **Karma** — a composite reading, recomposed on every read from several factors
  including hidden ones. It can go down. There is no maximum, no level, and no
  bar to fill; a single ascending level is the worldview the game critiques.
- **Biological age** — the headline health readout, composed on read from every
  biomarker including the ones the player cannot see. There is no chronological
  age in the game.
- **Environment** — a population, not a place. The rule set in force follows
  where the character lives, not where they are standing.
- **Familiarity / returns** — a decay multiplier, `exp(-(worked/420 + days/400))`
  clamped to `[0.02, 1]`, applied to positive authored karma in an environment.
  Costs are not discounted. At or below 0.5 the game begins pointing elsewhere.
- **The playbook** — a small fixed set of standing questions the world will ask.
  Each answer is a sentence composed from a closed vocabulary belonging to that
  question. None of the answers is correct; each is effective somewhere and
  ruinous elsewhere.
- **Stance** — every playbook action carries one of `yield / resist / bargain /
  defer / open / withdraw`. Currently held in content and never surfaced in UI.

**Hard product constraints**
- The game never teaches, never analyses the player's choices, and never states
  outcomes in advance.
- Locked features are never shown. The one deliberate exception is the body
  screen's purchasable instruments, where the design point is that the reading
  exists and the instrument is what the player lacks.
- Player-visible prose lives in `content/*.yaml` and is written only through the
  project's authoring pipeline. Components render; they supply no sentences.
  Numbers the mechanics own are injected by the UI, never written into prose.
- Content imitates its real-world counterpart, trimmed to game length. The
  literary register is banned.
- No multi-column layouts of two disconnected views. The one sanctioned
  exception is a list beside the single item opened from it, list on the left.
- No busywork: an action that reduces to "click to increment a stat" is cut.
- V1 priority is visible pathways and opportunities; gating comes later.

**Technical**
- Go backend (`cmd/`, `internal/`), React client (`web/`), authored content in
  `content/*.yaml`. Design tokens in `ui/tokens.css` split into a primitive
  palette layer and a semantic role layer; components read only the semantic
  layer.
- A day and a night plate exist per location as `.webp` under
  `web/public/locations/`.

## Brand Commitments

- Name: **Escape the Matrix**.
- Voice: plain, concrete, unliterary. Say what a thing is, never what it is not.
  No fluff; if a sentence can be cut without losing meaning, cut it.
- Owner: Michael Sheinman. `Documents/HighLevelOverview.md` is the settled
  high-level design; `Documents/Guidelines.md` is the binding rulebook and wins
  on conflict.

## Evidence on Hand

- Real authored content: `content/playbook/questions/*.yaml`,
  `content/jobs/wilmington/*.yaml`, `content/conflicts/`, `content/situations/`,
  `content/environments/`.
- Real world art: `web/public/locations/{wilmington,wilmington-apartment,
  riverfront,riverfront-rooms,house-fire}/plate.webp` and `plate-night.webp`.
- Real health model: `internal/health/health.go` (three tiers — baseline free,
  a good watch at $400, bloodwork at $950 — and eleven named metrics).
- Real decay model: `internal/simulation/factor.go`.
- The player avatar is the account's Google profile picture (`player.avatarUrl`).
- No monetisation, pricing, customers, testimonials, or benchmarks exist. None
  may be invented.

## Product Principles

1. **The mechanics carry the lesson.** Nothing is explained that could instead be
   lived through.
2. **Progress has no ceiling and no bar.** Any readout that implies a maximum
   contradicts the thesis.
3. **The right answer is local.** Every rule, reward, and reading is scoped to an
   environment, and the game never says which environment suits which rule.
4. **Things need to happen.** Visible pathways and opportunities over locked
   doors; the player is never bored waiting.
5. **The UI is part of the world.** The world keeps existing while the player
   reads about themselves.

## Accessibility & Inclusion

No product-specific standard has been established by the user. The existing
client uses keyboard hints (`ESC`, `P`, `M`) as a primary navigation affordance,
so keyboard reachability is an existing product behaviour to preserve.
