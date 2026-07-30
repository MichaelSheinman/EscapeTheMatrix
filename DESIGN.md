# Escape the Matrix — Design Document

**Status:** v0.2 — living document · **Started:** 2026-07-29 · **Owner:** Michael Sheinman

---

## 1. Pitch

A life simulator. You start at 17 and build a life — career, health, habits, money,
relationships, travel. The systems that make your character thrive are the same ones
that work in reality.

## 2. Positioning

**This is a game, not a growth platform.** Growth is a consequence of play, never the
label.

Two design rules:

- **Model, don't label.** Frameworks are expressed as characters and situations.
  Never named, never explained in-game.
- **No quizzes.** Nothing with a checkable right answer presented as a question.
  Understanding is demonstrated by what the world lets you do.

**The core idea that makes this work:** the simulation's reward function *is* the
curriculum. If sleep genuinely improves biomarkers, and biomarkers genuinely improve
job-interview outcomes, the player learns why sleep matters by optimizing — not by
being told.

Priority: **user growth first.** Monetization deferred entirely.

---

## 3. Game Model

### 3.1 Time and pacing — DECIDED

**Real-time and day-aligned. Actions resolve instantly.**

- One game day = one real calendar day. The clock is the player's own.
- **No waiting, ever.** No build timers, no energy refills, no "come back in four
  hours." Every action resolves immediately.
- **Actions don't occupy wall-clock slots.** The character can effectively be in two
  places at once — a 9-to-5 job does not block an 11 AM adventure.
- **Time of day is real and consequential.** Playing late degrades health. The game
  doesn't block it — it lets the player see the cost later, and can surface a
  suggestion.
- **Volume has consequences, not limits.** Many actions per day is allowed, but heavy
  stacking of *active* activities shows up as declining health.
- **Framing:** a long-term commitment that aligns with the player's real life — not a
  binge-hard-then-abandon loop.

### 3.2 Design rules

Constraints that any mechanic has to satisfy:

- **No busywork.** An action that reduces to "click to increment a stat" is dead
  weight. Wrap it in a storyline and a real choice, or cut it.
- **Don't score subjective interior states.** The game must not tell the player how
  happy they are — it should ask. Prefer observable proxies, e.g. an **emotional
  stability** indicator over a "happiness" number.
- **Progression is not a level bar.** A single ascending level is itself the worldview
  this game is critiquing. Progress needs a different shape. Open — see §6.
- **Stakes are required.** When nothing matters, players default to nihilistic play.
  Consequence is what makes a choice meaningful.
- **Instant ≠ trivial.** Fast to arrive, but the resolution still needs weight. Speed
  respects the player's time; it isn't a licence for thin content.
- **Breadth of lived experience is a feature.** Different countries, cultures, and
  value systems — a core draw, not decoration. (Tension with US-only V1 — see §6.)

### 3.3 The character

| Attribute | Notes |
| --- | --- |
| **Age** | **Cut. There is no chronological age.** See §3.8 |
| **Biological age** | Derived from health. The player's headline health readout |
| **Home** | A place they live. Shown on a grid; enterable |
| **Location** | Country + state. **US only for V1** |
| **Money** | Spendable; gates access to items and options |
| **Health** | A tab of biomarker metrics. Some metrics are **locked** until the player buys a good watch in-game |
| **Job** | Starts passive ("you have a job"). Later jobs require real activity |
| **Routines** | Player-configured schedules: sleep, workout, diet, organization |

### 3.4 Systems

**Routines & schedules.** The player configures sleep, workout, diet, and
organization routines. Early game focuses here and is hands-on; the arc is toward
"set it once and never worry about it again" so attention can move to bigger events.

**Struggle before mastery.** A routine cannot be configured until the player has
*experienced the problem it solves*. Struggle first, tool second. This is the primary
consciousness-raising mechanism — see §3.6, unresolved.

**The Plan.** The player writes a free-form plan in their own words. An LLM reads it
and translates it into concrete routines. Writing the plan is what unlocks health
configuration.

**Jobs.** Passive at first. Deeper roles carry real mechanics — e.g. a politician job
where the player writes laws and chooses which to enforce. **Getting hired spawns a
live AI interview** the player actually has to get through, rather than a dice roll.

**Economy — DECIDED.** Income is primarily **structural**: it comes from job tier,
performance, and assets, not from repeating actions. Actions build capability — skills,
health, reputation, relationships — which raises job tier, which raises income.

Some actions *do* pay directly, but the number of **money-producing actions per real
day is capped** (starting point: **3**). Past the cap, further earning is limited.

The cap applies only to earning, never to play — a player can still take unlimited
non-earning actions in a day. Whether the cap past 3 is a hard stop or a steep
drop-off is a tuning question, not a design one.

**Chat.** The character can talk to other people. V1 characters are AI; human-to-human
is V2 (see §3.7).

**Trips.** Solo or with other characters. Involve meeting cultures, playing games, and
hidden lessons.

**Military & combat.** An option to enlist and play fight games.

**Navigation.** A grid showing the home → enter the home → a map → the wider world.

**Guidance.** Text dialogue directs the player where to go. No voice in V1.

### 3.5 Progression and the leaderboard — DECIDED

**No level.** Progression is a set of factors, most of them **hidden from the player**.

**The leaderboard is distributed:** one overall board (a weighted mix of all factors),
plus a separate board for each individual factor.

| Factor | Grown by | V1 visibility |
| --- | --- | --- |
| **Health** | collective of all biomarkers — surfaced as **biological age** | **revealed** |
| **Money** | amount held | **revealed** |
| Intelligence | completing activities that demonstrate intelligence | hidden |
| Emotional stability | behavior in interactions; in-app chat history | hidden |
| Kindness | how helpful the player is; helpers can send energy; **peer endorsement** | hidden |
| Energy | spiritual state, via meditation challenges | **deferred to V2/V3** |

**The overall board gets an evocative name — working title "Karma."** The composite score
is what the player understands as growth. It replaces the level bar entirely.

**Money's contribution to Karma is a bell curve — DECIDED.** Early on, money genuinely
helps: doing work, providing value while poor, saving, and investing all raise Karma. Past
an ultra-wealthy point the relationship **inverts**, and further accumulation lowers it.

- **V1 builds the rising side and the plateau: diminishing returns above $1M.**
- **V2 builds the flip** — the downslope past ultra-wealth. Threshold and shape TBD.

```
KARMA
  ^
  |          .-''''''-.
  |        .'          `-.        <- V2: the flip
  |      .'                `-.
  |    .'                      `-.
  |  .'
  +-'--------------------------------> MONEY
   $0    $100k        $1M         $50M+
   |------ V1 --------|-- V2 ---------|
```

This curve is what makes the name "Karma" defensible. The game genuinely asserts that
hoarding past a point is bad for you, so the composite isn't a wealth proxy wearing a
spiritual label — and it can fall, which keeps it from being a level bar (§3.2).

**Health surfaces as biological age, not a score.** "Your biological age is 24" carries
more meaning than "Health: 72", is instantly legible without explanation, and is one of
the few numbers players are motivated to drive *downward*.

**The hidden factors are the hook.** The formula is never stated. *What are the hidden
factors? How are they calculated? What grows them?* Those questions are the thing that
keeps players thinking and experimenting. Health and Money are shown; the rest appear
as unnamed or locked slots.

**Challenges are the disclosure channel.** A challenge gives the player insight into how
to grow a factor without ever instructing them. The game never says "be kinder."

Implementation notes:

- Emotional stability and Kindness require **LLM evaluation of player behavior and chat
  history** — a scoring job in Go against Claude, not a rules formula.
- **Peer endorsement:** an occasional "I like this user" button players can press. Rate
  it, don't make it always-available. This is *asynchronous* social — it needs no
  real-time interaction, so it can ship in V1 alongside leaderboards, which already
  imply seeing other real players. It also gives Kindness a real human signal in V1
  rather than an AI-only proxy.
- Sending energy and direct helping still depend on richer player-to-player interaction
  (V2, §3.7); in V1 those register against AI characters.
- Peer endorsement needs abuse resistance (reciprocal like-rings, alt accounts). Rate
  limiting plus weighting by the endorser's own standing is the usual answer.
- **Risk to watch:** if a player can't tell what moves a hidden number, the board reads
  as noise rather than mystery. Challenges are the mitigation — needs playtesting.

### 3.6 Struggle-gated unlocking — DECIDED

**Mechanism: diagnosis.** The player gathers evidence, then names the cause of a
failure. Naming the cause is what grants the tool. The insight *is* the gate.

This is the purest expression of the theme, and it carries a real risk of reading as a
quiz — which §3.2 bans. Accepted knowingly. These five rules are how we keep it from
becoming one:

1. **Point at evidence, not at answer strings.** No four authored options. The player
   selects from their own data — sleep log, calendar, chat history, biomarkers. The
   interaction is investigative, not multiple choice.
2. **Never say "correct."** No right/wrong feedback, ever. The player acts on their
   diagnosis and the world's response reveals whether they were right.
3. **Wrong diagnoses are playable, not blocked.** Conclude "she just didn't like me" and
   the game lets you chase that — you train charisma, and it doesn't fix anything,
   because the cause was sleep. The failure is the lesson. No retry loop, no buzzer.
4. **Allow multiple valid causes.** Real failures are multi-causal. Several diagnoses
   can be partly right, unlocking different tools of differing usefulness. There is no
   single key.
5. **Re-diagnosable.** The player can revisit a past conclusion once they have more
   evidence.

**Delivery: a scene, not a screen.** Stage the diagnosis as a conversation with a
character — coach, doctor, older mentor — at the moment of consequence. This removes the
form-filling feel and reuses the AI dialogue system.

**Open:** this supersedes the Plan (free-form text → LLM → routines) *as an unlock
gate*. Does the Plan survive as an optional tool afterward, or is it cut?

### 3.7 Social

- **V1:** players interact mostly with AI characters. Plus **leaderboard stats**.
- **V2:** human collaboration and player-to-player interaction. Design the data model
  so this is possible later, but do not build it now.

### 3.8 No age, no ending — DECIDED

**Chronological age is cut.** It was genre convention, not a mechanic, and it worked
against the game's goals — a rising age number is another linear ladder (§3.2), and at one
game day per real day it would have crawled meaninglessly.

Consequences:

- **Growth is the composite score ("Karma")**, plus the individual factor boards (§3.5).
  That is the whole progression model.
- **Biological age is shown** as the health readout. It is not a life-stage counter.
- **No ending, no age cap, no restart.** The game runs alongside the player indefinitely,
  matching the long-term-commitment framing (§3.1).
- The character begins as a young person choosing a direction — that stays as *narrative
  context*, not as a tracked, advancing number.
- **Account age** exists as a fact but has no confirmed use. Do not surface it without a
  reason; it rewards seniority rather than growth, which cuts against the whole model.

### 3.9 Challenge delivery — DECIDED

**Diegetic. The world offers the challenge. There is never a recommendation UI** — no list,
no queue, no suggestion feed. Challenges arrive because of where the player is and what
state they're in: they pass an argument, a friend calls, an interview exists because they
applied.

- **Selection is server-side and LLM-assisted.** Claude reads player state, history, hidden
  factors, and chat history to choose what would genuinely serve this player next —
  including things they would avoid if given a menu. The presentation stays diegetic; the
  selector is invisible.
- **Selection inputs:** location, current activity, which factors are lagging, time since
  the last challenge of each register, which characters the player already knows, and what
  they have been declining.
- This is the same move as the hidden leaderboard factors (§3.5) — the mechanism is real and
  the player never sees it.

Implementation notes:

- Challenges are authored with **placement rules** — the contexts they may legitimately
  appear in (on foot downtown, at home late, at work) — so the selector never places one
  absurdly.
- The player must be able to **decline without consuming the challenge**. Walking past the
  argument returns it to the pool rather than burning it.
- **Declining is signal.** A player routing around every interpersonal encounter is telling
  the selector something worth acting on.
- A **debug view** is required: which challenge was selected, why, and what state drove it.
  LLM-assisted, non-deterministic selection is otherwise close to untestable.

---

## 4. Graphics Direction

**2D, isometric or 3/4 perspective.** The core loop is management, dialogue, and
navigation — a 3D camera adds little, while a 3D pipeline (rigged characters,
animation state machines) would consume most of the art budget. 3D on the web is
feasible (Three.js, WebGPU); we decline it on budget allocation, not capability. A
single 3D set-piece can be added later without changing the stack.

**Pre-rendered fidelity, not flat 2D.** Build assets at high fidelity offline, bake to
2D layers, composite with real-time lighting, particles, and post-processing.

### Art bible — LOCKED

**Register: grounded contemporary, painterly.** Real places rendered warmly and
specifically. Recognition over spectacle — the player should feel they have been in this
room. Premium comes from lighting and specificity, never from fantasy or scale.

| | Rule |
| --- | --- |
| **Camera** | 3/4 isometric, close. **One fixed angle across every scene, no exceptions** — inconsistent camera is the fastest way to make generated assets look unrelated |
| **Palette** | Warm neutrals as the base; sodium amber as the dominant warm; cold window blue as the single contrast. 10–12 swatches total. No scene introduces a hue outside it |
| **Lighting** | One dominant source per scene, heavy falloff. Genuine unlit areas are wanted — darkness is allowed. Light carries the mood; geometry stays plain |
| **Detail** | Specificity over density. One or two telling props beat a cluttered room. Every scene needs at least one detail implying a person lives there |
| **Proportions** | Realistic. No stylised heads. Portraits carry likeness; world sprites need only silhouette readability |
| **UI** | Shares the palette and the warmth. The UI is part of the world, not a dashboard bolted onto it — dense panels get the same logic: warm ground, one accent |

**Proposed — reserve digital-surreal language for moments of insight. Confirm or cut.**
Keep the world entirely grounded, and use restrained constructed-reality treatment — a
brief grid, a tear at the edges — *only* when the player diagnoses something correctly
(§3.6) or a hidden factor shifts (§3.5). That earns the title without theming the whole
game, and makes insight visually distinct from every other moment in it.

### Art pipeline

The bottleneck isn't generating art — it's generating art that looks like it came from
one game.

1. **Art bible first, locked, one page.** Camera angle, palette, lighting rules, line
   weight, detail level, character proportions. Every asset checked against it.
2. **Backgrounds:** generate above target resolution, hand-correct perspective. Reuse
   heavily — a life sim revisits the same locations constantly.
3. **Characters, two tiers:** high-fidelity *portrait* for dialogue (what the player
   stares at); simple *world sprite* that only needs to read clearly at small size.
4. **Style consistency:** LoRA trained on our own approved outputs, or locked seed +
   style reference. Revisit at ~20 approved assets.
5. **Motion sells more than resolution.** Dedicated juice pass — parallax depth, rim
   lighting, particles, breathing/blink idles, bloom, vignette, easing on every
   transition. Highest-ROI item in the art plan.

**Note:** a life sim is UI-heavy — health tabs, schedule editors, inventory, job
screens, leaderboards. That UI needs as much art direction as the world does, and it
is where most of the player's time is actually spent.

---

## 5. Tech Stack

**Backend is Go.** The browser only executes JS and WASM, so a client layer is
unavoidable — but game logic does not have to live there.

| Layer | Choice | Notes |
| --- | --- | --- |
| Backend | **Go** | Owns simulation, game logic, state, AI orchestration, persistence |
| HTTP / routing | `net/http` stdlib | Go 1.22+ router is sufficient; minimal deps |
| Realtime | WebSocket (`coder/websocket`) + SSE | SSE for AI token streaming |
| AI characters | `anthropic-sdk-go` v1.61.0 | Official SDK; `claude-opus-5`, streaming, adaptive thinking |
| Persistence | **Postgres** via `pgx` | Character state, routines, progression, leaderboards |
| World renderer | **PixiJS v8** | WebGL2/WebGPU 2D renderer, in the browser |
| UI / dialogue / tabs | **React (DOM)** | A life sim is mostly UI; DOM is the right tool |
| Client language | TypeScript | Rendering, input, presentation only — no game logic |
| Asset serving | Go `embed.FS` | One binary serves API + built frontend |
| Hosting | Fly.io / Railway / Cloud Run | Go binary; not a Vercel fit |

### Why this split

Go stays authoritative over all rules and the simulation. The client renders what the
server sends and forwards input. TypeScript exists as a rendering layer, keeping game
logic in one language.

**Deliberately rejected: Go → WASM via Ebitengine** (whole game in Go). Ebitengine is
mature and compiles to WASM, but renders everything into a canvas — no DOM or CSS UI.
For a UI-heavy life sim that is the wrong trade. Large binaries, and TinyGo can't
shrink them well due to reflection use.

**Planned evolution: shared rules package.** Structure game rules as a standalone Go
package so it compiles natively (server, authoritative) *and* to WASM (client, local
prediction for snappy UI without a round trip). Rules written once, in Go. Build for
this shape now; enable it when latency demands it.

### Platform — DECIDED

**Desktop web first.** The game has genuinely dense systems — biomarker panels, schedule
editors, leaderboards, an explorable isometric world — and desktop is where all of it can
be visible at once and where the graphics can actually be impressive. Accepted cost:
narrower reach up front, and mobile becomes a retrofit rather than a port.

Three cheap habits that keep the eventual mobile retrofit from being brutal. All are
free on a desktop-first path if adopted now, and expensive to add later:

1. **No hover-only interactions.** Anything reachable only by hover is unreachable on
   touch. Hover may reveal detail, never function.
2. **Layouts driven by flex/grid, not pixel positions.** Panels reflow instead of being
   pinned.
3. **Keep the world canvas independent of UI chrome.** The Pixi viewport shouldn't assume
   the panels around it — so panels can later collapse or stack without touching the
   renderer.

### Setup prerequisites

- Go toolchain — **not currently installed** on this machine
- Node 22.2.0 ✅ · npm 10.7.0 ✅
- Postgres instance (local Docker for dev)
- Anthropic API key

---

### 3.10 Challenges — the primary content system

**A challenge is a big endeavour, not a small interaction.** Roughly **20 minutes** to
complete. Each has its own pathways and interactions, and some have their own mechanics
entirely. Many will be needed. This is the game's main content and its largest production
cost.

**Building a single challenge is a project in its own right.** For planning purposes each
challenge is an **epic**, not a ticket — it carries its own design, art, dialogue, scoring
rubric, and sometimes its own mechanics. The challenge *framework* (§3.11) is a separate
piece of work from any challenge built on it, and it should exist first.

**V1 archetype is developmental, not mechanical — DECIDED.** "Archetype" here means the
*developmental register* of a challenge, not its interaction style. **V1 focuses on
Orange- and Green-register challenges** — achievement, career, strategy, negotiation,
learning; and empathy, relationships, conflict, community, cross-cultural understanding.
**Red-register material — war, combat, fighting, dominance — is deferred to V2+.**

Within that focus V1 has **multiple challenge types with different mechanics.** V1 is not
restricted to one interaction style; it is restricted by register. The conflict-resolution
example (Green) and the AI job interview (Orange) are both in V1 range. The war challenge
and its shooting mode (Red) are not.

> Stage names are **internal production vocabulary only**. Per §3.2 the game never labels
> a framework for the player, and never displays a register.

**V1 roster: 6–8 challenges — DECIDED.** Roughly four Green (interpersonal conflict,
family, community, cross-cultural) and four Orange (interview, negotiation, investment,
examination), across **3–4 distinct mechanics**. V1 launches as a real game with weeks of
content rather than a demo.

> **Sequencing, to de-risk the roster:** build the framework and *one* challenge all the way
> to playable before starting the other five to seven. The framework's real shape only
> becomes visible once something complete runs on it, and reworking it after seven
> challenges exist is far more expensive than after one.

A challenge composes from a shared kit but is individually authored:

- a multi-stage scenario with real branching
- character dialogue that reacts to the player's actual interpretation
- **free-text input** where the player commits a judgement in their own words
- **hidden scoring** on the quality of that response — never surfaced as a grade
- optionally, an embedded **education** segment
- optionally, a dedicated **mode** with its own mechanics
- rewards spread across several factors

**Worked example — resolve a conflict.** You pass two people arguing; say a husband and
wife. They ask for your help and lay out all the details. You weigh it and give advice,
including free text. You are effectively playing therapist. Hidden: you are ranked on the
quality of your response. The characters then react according to how they *perceive* your
interpretation.

**Worked example — go to war.** Includes genuine education on how wars work. You select a
weapon and have to understand which one suits the situation. You talk to people in combat.
Then a separate **shooting mode** with its own mechanics. You cannot die, but you can be
injured. Survive and you earn money, intelligence, and more.

### 3.11 What §3.10 implies for the build

1. **Challenges must be a plugin system, not engine features.** With bespoke pathways and
   sometimes bespoke mechanics, they cannot be hardcoded into one loop. This needs a
   challenge framework: shared services (dialogue, scoring, rewards, state, save/resume)
   plus per-challenge logic. In Go, a challenge interface and registry; on the client,
   per-challenge scene modules loaded on demand.
2. **Each distinct mechanic is its own subsystem.** V1 spans multiple challenge types, so
   this is the main variable cost inside challenge work — a new interaction style is far
   more expensive than another challenge in an existing style. Deferring the Red register
   removes the single most expensive mechanic (real-time combat: input, collision,
   animation, feel-tuning — close to a second engine). Keep the framework from assuming
   turn-based interaction so it can host action later.
3. **Hidden response scoring is an LLM job with a rubric.** Per challenge: a rubric, a
   structured score, and an in-character reaction. Claude with structured output. The score
   feeds hidden factors and is never shown to the player as a grade.
4. **Reuse is what makes this affordable.** Challenges should share locations, characters,
   and mechanic archetypes even when their pathways differ. Without reuse, twenty
   challenges is twenty separate games.
5. **Content burn rate is a real risk.** A bespoke 20-minute challenge is largely
   single-use. Either accept a high ongoing authoring rate, or build variation
   (parameterised specifics, AI-generated detail) into each challenge shell.
6. **Save and resume is mandatory.** Twenty minutes is long enough that players will
   abandon partway through.

### 3.12 Health and biomarkers — DECIDED

**Two layers: a few visible drivers, and a deep simulated panel revealed by
instrumentation.**

The underlying simulation always runs the full model. What instrumentation changes is the
player's **resolution on their own body**, not their health. Buying a watch doesn't make you
healthier — it makes you able to see.

| Tier | Unlocked by | Metrics (indicative) |
| --- | --- | --- |
| **Baseline** | free, from the start | sleep hours, resting heart rate, weight trend, biological age |
| **Wearable** | buying a good watch | HRV, sleep stages, recovery, VO2 max, activity load |
| **Clinical** | bloodwork / medical access | glucose stability, inflammation markers, lipids, hormonal markers |

Rules:

- **Every metric is a real-world concept.** They teach genuine ideas and can be tuned against
  real relationships — sleep debt → HRV collapse → degraded performance under pressure.
- **The hidden simulation is authoritative.** A player without a watch still suffers real HRV
  collapse; they simply can't see the cause. That *is* the diagnosis problem (§3.6) — buying
  instrumentation is one legitimate route to becoming able to diagnose.
- **Biological age is the composite readout**, visible from day one at every tier.
- Instrumentation is a **money sink that buys insight**, which ties the economy (§3.4) to the
  learning loop rather than to power.

**This closes a loop worth naming:** money buys instrumentation, instrumentation enables
diagnosis, diagnosis unlocks routines, routines improve health. It is also the mechanical
justification for money's *rising* contribution to Karma (§3.5) — early money genuinely does
buy the ability to see and act.

**Proposed — imprecise biological age at low resolution. Confirm or cut.** At baseline the
figure could be shown as a range ("29–34") and narrow as instrumentation improves. The player
can always see it, but knowing it *precisely* is itself something to earn.

## 6. Scope — DECIDED

**V1 is everything in this document except the items explicitly marked V2 or later.**
No slicing, no phased core, no minimum viable version.

**This is deliberately a giant project — on the order of a year of work.** The ambition is
the point: a stripped-down version of this game would be indistinguishable from the shallow
life-sims it exists to answer. Scope reduction attacks the premise rather than de-risking
it. Sequencing decisions that reduce rework are welcome; cuts are not.

Once investigation is complete, V1 gets broken into tickets and worked one at a time.

Deferred out of V1:

| Deferred | To |
| --- | --- |
| Human collaboration / player-to-player interaction | V2 |
| Sending energy, direct helping between real players | V2 |
| Energy factor + meditation challenges | V2/V3 |
| The Karma "flip" — money's downslope past ultra-wealth (§3.5) | V2 |
| Red-register challenges — war, combat, fighting — and shooting mode (§3.10) | V2+ |
| Any 3D set-piece | later, optional |
| Shared Go rules package compiled to WASM for client prediction | later, when latency demands it |

**Peer endorsement ("I like this user") stays in V1** — it is asynchronous, needs no
real-time interaction, and gives Kindness a real human signal from day one (§3.5).

**Cultural breadth in V1 — working resolution, confirm.** V1 is based in the US (§3.3),
and breadth of lived experience is a core draw (§3.2). These reconcile through **trips**
(§3.4): the player is US-based but travels, and trips are where other cultures and value
systems are encountered. Alternate *starting* lives — beginning the game as an indigenous
person, or in a stage-red community, or in another country — are the V2 expansion of the
same idea.

## 7. Open Requirements

Unresolved, blocking implementation:

- **World and map structure** — how many locations, what a city contains, how freely the map
  is explored, and how the home-grid → interior → map layers connect. Now load-bearing for
  §3.9, since diegetic delivery depends on the player being somewhere specific.
  **Largest open question.**
- **Biomarker tuning** — the actual formulas: how routines and actions move each metric, and
  how the panel composes into biological age (§3.12).
- **Trips** — barely specified beyond "meet cultures, play games, hidden lessons" (§3.4).
- **Organization routine** — named as a valuable habit to teach (§3.4) but undefined as a
  mechanic.
- **Jobs** — the progression from passive job to active roles, and what active work looks
  like beyond the politician example.
- **Chat** — structure, and how it feeds emotional-stability scoring (§3.5).
- **Onboarding and the first session** — with no age and no ending, the opening carries the
  whole job of establishing what the game is.
- **Money: amount or source?** The curve (§3.5) is drawn against *amount held*, but the
  reasoning behind it was partly about *how* money is earned — putting in work, providing
  value. Does earning method modulate the contribution, or only the balance?
- **Fate of the Plan** — kept as an optional tool or cut, now that diagnosis is the gate
  (§3.6).
- **Accounts** — anonymous instant play vs. login from day one (leaderboards and peer
  endorsement both imply identity).
