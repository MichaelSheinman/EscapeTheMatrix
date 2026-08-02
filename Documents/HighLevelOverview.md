# Escape the Matrix — Design Document

**Status:** v1.0 — **high-level design complete** · **Owner:** Michael Sheinman
**Started:** 2026-07-29 · **Last session:** 2026-07-30

> **Where this stands.** The high-level design is settled — see the DECIDED and LOCKED markers
> throughout §3–§6. Implementation detail lives in `technical.md`; §7 lists what is still open.

---

## 1. Pitch

A life simulator. You build a life — career, health, habits, money, relationships, travel.
The systems that make your character thrive are the same ones that work in reality.

## 2. Positioning

**This is a game, not a growth platform.** Growth is a consequence of play, never the
label.

- **Model, don't label.** Frameworks (e.g. spiral dynamics) are expressed as characters and situations.
  Never named, never explained in-game.

**The core idea that makes this work:** the simulation's reward function *is* the
curriculum. If sleep genuinely improves biomarkers, and biomarkers genuinely improve
job-interview outcomes, the player learns why sleep matters by optimizing — not by
being told.

### The thesis the game exists to demonstrate

**Different situations call for different appropriate actions.** There is no universal correct
rule set — the best rules in one environment are the wrong rules in another. The game proves
this by letting a player master one environment, hit its ceiling, travel, and discover that
their hard-won rules fail completely somewhere else.

> **The matrix, in this game's terms, is mistaking your local rule set for universal truth.**
> Escaping it means holding several, and knowing which one applies where.

This is not a message delivered to the player. It is what the mechanics do (§3.15).

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
  this game is critiquing. Progress needs a different shape — see §3.5.
- **Stakes are required.** When nothing matters, players default to nihilistic play.
  Consequence is what makes a choice meaningful.
- **Instant ≠ trivial.** Fast to arrive, but the resolution still needs weight. Speed
  respects the player's time; it isn't a licence for thin content.
- **Breadth of lived experience is a feature.** Different countries, cultures, and
  value systems — a core draw, not decoration. Reached through trips in V1 — see §5.

### 3.3 The character

| Attribute | Notes |
| --- | --- |
| **Age** | **Cut. There is no chronological age.** See §3.8 |
| **Biological age** | Derived from health. The player's headline health readout |
| **Home** | A place they live. Shown on a grid; enterable. §3.12 |
| **Location** | One US city, one district. §3.12 |
| **Money** | Spendable; gates access to items, options, and instrumentation |
| **Health** | Two-tier biomarker panel; deeper metrics unlocked by instrumentation. §3.16 |
| **Job** | Tiered. Low tiers passive, high tiers delegated via the playbook. §3.14 |
| **Routines** | Player-configured schedules: sleep, workout, diet, organization |

### 3.4 Systems

**Routines & schedules.** The player configures sleep, workout, diet, and
organization routines. Early game focuses here and is hands-on; the arc is toward
"set it once and never worry about it again" so attention can move to bigger events.

**Struggle before mastery.** A routine cannot be configured until the player has
*experienced the problem it solves*. Struggle first, tool second. This is the primary
consciousness-raising mechanism — see §3.6.

**Jobs.** Tiered, with senior roles delegated through the playbook — see §3.14. **Getting
hired spawns an interview challenge** the player actually has to get through, rather than a dice
roll — authored and deterministic like every other challenge (technical.md §5). **A live AI
interview is a V2 addition.**

**Economy — DECIDED.** Income is primarily **structural**: it comes from job tier,
performance, and assets, not from repeating actions. Actions build capability — skills,
health, reputation, relationships — which raises job tier, which raises income.

Some actions *do* pay directly, but the number of **money-producing actions per real
day is capped** (starting point: **3**). Past the cap, further earning is limited.

The cap applies only to earning, never to play — a player can still take unlimited
non-earning actions in a day. Whether the cap past 3 is a hard stop or a steep
drop-off is a tuning question, not a design one.

**Chat.** The character can talk to other people. V1 characters are NPCs running authored
dialogue trees, not generated text; human-to-human is V2 (see §3.7).

**Trips.** Solo or with other characters. Meeting cultures, playing games, hidden lessons.
**Now the primary progression vehicle** — travel is the only route past an environment's
ceiling (§3.15), not a side activity.

**Military & combat.** An option to enlist and play fight games. **Red register — deferred to
V2+** (§3.10, §5).

**Navigation.** Home grid → interior → district → city map. See §3.12.

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

Notes:

- **Peer endorsement:** an occasional "I like this user" button players can press. Rate
  it, don't make it always-available. This is *asynchronous* social — it needs no
  real-time interaction, so it can ship in V1 alongside leaderboards, which already
  imply seeing other real players. It also gives Kindness a real human signal in V1
  rather than an AI-only proxy.
- Sending energy and direct helping still depend on richer player-to-player interaction
  (V2, §3.7); in V1 those register against AI characters.
- **Risk to watch:** if a player can't tell what moves a hidden number, the board reads
  as noise rather than mystery. Challenges are the mitigation — needs playtesting.

### 3.6 Struggle-gated unlocking — DECIDED

**Mechanism: diagnosis.** The player gathers evidence, then names the cause of a
failure. Naming the cause is what grants the tool. The insight *is* the gate.

This is the purest expression of the theme, and it carries a real risk of reading as a
quiz. Accepted knowingly. These five rules are how we keep it from becoming one:

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
form-filling feel and reuses the authored dialogue system.

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

- **Selection is server-side and deterministic.** A heuristic reads player state, history, and
  hidden factors to choose what would genuinely serve this player next — including things they
  would avoid if given a menu. The presentation stays diegetic; the selector is invisible. No
  model in the loop, and nothing the player writes is read (technical.md §7).
- **Selection inputs:** location, current activity, which factors are lagging, time since
  the last challenge of each register, which characters the player already knows, and what
  they have been declining.
- This is the same move as the hidden leaderboard factors (§3.5) — the mechanism is real and
  the player never sees it.

Notes:

- Challenges are authored with **placement rules** — the contexts they may legitimately
  appear in (on foot downtown, at home late, at work) — so the selector never places one
  absurdly.
- The player must be able to **decline without consuming the challenge**. Walking past the
  argument returns it to the pool rather than burning it.
- **Declining is signal.** A player routing around every interpersonal encounter is telling
  the selector something worth acting on.

### 3.10 Challenges — the primary content system

**A challenge is a big endeavour, not a small interaction.** Roughly **6 minutes** to
complete. Each has its own pathways and interactions, and some have their own mechanics
entirely. Many will be needed. This is the game's main content and its largest production
cost.

**Building a single challenge is a project in its own right.** For planning purposes each
challenge is an **epic**, not a ticket — it carries its own design, art, dialogue, scoring
rubric, and sometimes its own mechanics. The challenge *framework* (technical.md §4) is a separate
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
- **a committed judgement** — the player selects the interpretation they will act on from a set of
  authored positions. There is no correct one; each leads somewhere different
- **hidden scoring** — every option carries authored factor deltas. Never surfaced as a grade,
  never confirmed as right
- optionally, an embedded **education** segment
- optionally, a dedicated **mode** with its own mechanics
- rewards spread across several factors

**Worked example — resolve a conflict.** You pass two people arguing; say a husband and
wife. They ask for your help and lay out all the details. You weigh it and give advice by choosing
which reading of the situation you commit to. You are effectively playing therapist. Hidden: your
choice moves several factors. The characters then react according to how they *perceive* your
interpretation.

**Worked example — go to war.** Includes genuine education on how wars work. You select a
weapon and have to understand which one suits the situation. You talk to people in combat.
Then a separate **shooting mode** with its own mechanics. You cannot die, but you can be
injured. Survive and you earn money, intelligence, and more.

**Worked example - jury dury** You go on Jury Duty to attend a trial for a given individual.
You are presented evidence, and need to make a decision about whether the individual 
is innocent or guilty. 

> **Free-text judgement is a V2 addition.** Authored options keep V1 fully deterministic —
> consistent, testable, instant, and with no model in the loop. The challenge schema reserves a
> free-text node type so it can be added later without reshaping anything (technical.md §7).

### 3.12 World structure — DECIDED

**One city, built deep, freely walkable.** Depth over breadth — a single city rendered
properly, with districts that have real character, walkable streets, and enterable interiors.

- **Districts with distinct character** — e.g. downtown (offices, transit), east side (the
  player's apartment), riverfront (where money lives), the strip (warehouses, night work).
- Roughly **6–10 hand-built enterable interiors per district**, all made to the art bible (§4).
- The player **walks** between locations, or takes transit.
- Layers: home grid → interior → district → city map.

**Walking is the encounter surface, not a wait.** This is the key synthesis with §3.9: the
reason to render a walkable city at all is that traversal is *where diegetic challenges
appear*. Movement costs no game time (§3.1), but it is where things happen to you. A player who
always takes the fastest route sees less of their own life.

**Proposed — walk vs. transit as a real trade. Confirm or cut.** Neither costs game time, so
the trade isn't speed: walking is free, generates encounters, and adds activity load (good for
health); transit costs money and skips encounters.

**Consequence:** with one city, every interior is reused heavily across challenges — which is
what makes a 6–8 challenge roster affordable (technical.md §4) and lets the city become genuinely
familiar the way a real place does. Other cities and countries arrive through trips (§3.4) and
later expansion.

### 3.13 Onboarding — DEFERRED

**V1 assigns a random avatar. There is no character creation flow yet.** Onboarding design is
deliberately parked so effort goes into the rest of the game; the placeholder is a randomly
generated character and a default starting situation.

Constraints for whenever it does get designed:

- **Character building, not intake.** They invent a person; fabricated details are legitimate, 
and the gap between player and avatar is part of the appeal.
- **Relationships are earned, not declared.** Nothing asks who the player has in their life —
  the character meets people in the world.
- **Not a form.** Every answer should visibly change the starting state rather than be cosmetic.
- **Appearance implies a portrait system.** Portraits are high-fidelity (§4), so any creation
  flow needs layered portrait assets or a curated roster — an art-production requirement, not a
  slider panel. The random-avatar placeholder needs the same roster, so this work arrives early
  regardless.

**Still open:** the fate of the Plan (§3.4). Its most likely home was a creation question
("what your character is after"), which is now parked with the rest of onboarding.

### 3.14 Jobs and the playbook — DECIDED

**Jobs sit in tiers.** Low tiers are genuinely passive — you hold the job, it pays, and it costs
you health if your routines are bad. Higher tiers pay far more and demand real involvement.
Each step up requires capability plus an interview (§3.4).

| Tier | Example | Involvement |
| --- | --- | --- |
| 1 | warehouse nights | passive; pays little; wrecks sleep |
| 2 | dispatch coordinator | passive, occasional decision |
| 3 | operations manager | active: staffing, budget, someone quits |
| 4 | regional director | active, with multi-week consequences |
| 5+ | politician, founder | active: write laws, choose what to enforce |

The ladder itself teaches the trade: more money costs more of your attention.

**Active jobs are delegated, not micromanaged.** The player does not sit inside their job. The
role runs on its own and generates interactions while they are elsewhere. The loop:

1. **It happens without you.** Situations arise in the role — a staffing call, a budget choice,
   someone quits — and the character handles them.
2. **You review.** When the player checks their analytics and wants to improve their standing,
   they read back what happened and how their character handled it.
3. **You compose rules.** The player builds a **playbook** — `when <condition> then <action>`,
   composed from a vocabulary of conditions and actions they unlock through play (§3.6).
4. **A rule fails.** The game produces a situation the rule covers badly, or doesn't cover.
5. **You revise.** Back to step 3 — permanently.

The loop never closes. The player is continuously reconsidering which rules are right and which
skills are worth acquiring.

**This is the game's thesis as a system.** The player is never handed good heuristics. They
author their own, watch reality break them, and improve. Nothing is labelled (§3.2), the world's
response is the grade, and the failure of a rule you wrote yourself is the most legitimate
teacher available.

**The playbook generalises.** Routines (§3.4) are already rules — rules for the body. The playbook
is rules for judgement. Same concept, same interface language, one system.

Notes:

- **The vocabulary is the design.** The set of available conditions and actions defines the space of
  thinkable strategies. New terms are unlocked through struggle-gating (§3.6), so the playbook grows
  in expressiveness as the player earns it.
- **Over-general rules must backfire, not win.** Players will compose "always maximise X." The
  correct response is a situation where exactly that rule costs them something — which is now
  something to author deliberately rather than hope for.
- **Failure must be legible.** Because a rule is composed rather than written in prose, the player
  can see exactly which condition matched and which action fired. That is what makes transfer
  failure diagnosable (§3.15) rather than a matter of interpretation.
- Good rules demonstrate judgement, feeding **Intelligence** and **Emotional stability** (§3.5).

### 3.15 Environments and rule-set transfer — THE CORE PROGRESSION

**Progression is lateral, not vertical.** The player does not climb — they widen. This is the
mechanism that makes the thesis (§2) into gameplay, and it is the spine the rest of the design
hangs from.

The loop, at the scale of the whole game:

1. **Master an environment.** The player refines their playbook (§3.14) until it works well
   where they are.
2. **Hit the ceiling.** Each environment has a real, visible plateau. Maxing it stops paying.
3. **Travel.** Trips (§3.4) are the only route onward — not a side activity, the primary
   progression vehicle.
4. **Watch the rules fail.** In a different environment, the old rule set doesn't underperform —
   it **doesn't work at all**.
5. **Build a second rule set** for the new environment.
6. **Mastery is plural.** Holding several context-appropriate rule sets, and knowing which one
   applies where.

**Ceilings are economic, plus explicit encouragement — DECIDED.** Two mechanisms working
together:

1. **Decay (the pressure).** Karma gains from an environment decay the longer the player stays and
   the more thoroughly they have mastered it. Same effort, shrinking return.
2. **In-app recommendations (the pointer).** The game actively suggests travelling — it doesn't
   rely on the player inferring it from flat numbers alone.

**Travel is intrinsically appealing**, so this is pull rather than push: decay removes the reason
to stay, the recommendation points somewhere, and wanting to go does the rest.

A recommendation suggests the trip; it never explains the lesson. *"You've been here a while —
there's a crew out past Zanesville"* — never *"your rules only work here."* Per §3.2, the
mechanism stays unlabelled.

```
KARMA FROM THIS ENVIRONMENT
  week 1   +240
  week 2   +180
  week 3    +95
  week 4    +40
  week 5    +12
  week 6     +4
```

> **This is the game's consistent teaching language.** Money's contribution flattens past a
> threshold (§3.5); an environment's contribution decays as it is mastered. Both teach by making
> returns fall rather than by explaining anything. Reach for this shape whenever a lesson needs
> delivering.

Notes:

- Decay is **per environment and per population** — relocating restores the rate.
- Decay should track **mastery**, not merely elapsed time; otherwise a player who barely engages
  gets pushed out as fast as one who genuinely mastered the place.
- Returns decay but **never go negative**. Staying may be unrewarding; it should not be punitive.

#### How the factors behave across environments — DECIDED

**Breadth pays through the individual factors, not through a global multiplier.** Karma is a mix
(§3.5), and several of its components are **environment-scoped** — capped, throttled, or boosted
by where the player is and how long they've been there. No separate "versatility" score is
needed; the existing factors already encode the thesis.

| Factor | Staying in one environment | Moving to a new one |
| --- | --- | --- |
| **Intelligence** | **capped.** Plateaus once the rule set works | **rises** as the player rebuilds a rule set for different people |
| **Money** | **capped by that environment's economic ceiling** — a Green setting affords less | **more options.** An Orange setting has a higher ceiling |
| **Energy** (V2/V3) | **declines** the longer the player is stuck in one environment or job | **boosted** by a new environment |
| **Health** | **no environment effect in V1** | no effect |
| **Emotional stability** | **capped by situational variety.** One environment offers a limited range of interactions | new kinds of situation to respond to |
| **Kindness** | same — capped by the range of chances to help | new opportunities to help |

**Emotional stability and Kindness are not caused by the environment — they are limited by its
exposure.** The relation is not causal: an environment does not raise or lower them. They grow from
*interactions*, and growth needs data from a variety of situations. A single environment contains a
finite repertoire, so once the player has seen its situations the factor stalls — not because the
place is bad for them, but because there is nothing new left to respond to.

**Intelligence is the adaptive-capacity factor.** It does not measure knowledge — it measures the
ability to work out that different people need different rules, and to build the new set. That is
why it cannot grow without moving.

**Environments are partly characterised by their factor affordances.** "Green setting, low money
ceiling, high relational reward" versus "Orange setting, high money ceiling, punishes trust" is a
real design axis for authoring a population, alongside its values.

**Three pressures now push outward at once**, which is why no single one has to be heavy-handed:
Karma decay per environment (above), Intelligence capping, and Energy decline — plus explicit
travel recommendations.

Design requirements this imposes:

- **Rule sets are environment-scoped, not global.** The playbook is not one list; it is a set of
  context-tagged rule sets. The interface has to make that structure obvious.
- **Ceilings must be real and legible.** A player who has maxed an environment needs to *feel*
  the plateau, or they will grind instead of travelling.
- **Transfer failure must be diagnosable, not random.** When a rule fails in a new environment
  the player must be able to work out why (§3.6). Arbitrary failure teaches nothing.
- **Karma should reward breadth over depth** — several working rule sets beat one perfect one.
  This is a real constraint on how the composite is weighted (§3.5).
- **Environments are never labelled.** No stage names, no framework vocabulary, ever (§3.2,
  §3.10). They are simply places where different things work.

**V1 scope:** the Orange and Green registers (§3.10). Two environments is enough to prove the
mechanic — the first plateau-and-transfer *is* the lesson. Further registers arrive as content
in V2+.

#### Trips are the vehicle — a second home, not an excursion

**A trip is a relocation.** The player goes somewhere and *lives* there across real days — local
work, local routines, local people. Long enough that the imported rule set visibly fails and a
second one has to be built in place.

**An environment is defined by its people, not its geography.** This is the key insight: what
changes is *who you are among* and what they value. A trip needs no new country and no new
culture — it needs a different population with a different value system. This is true to
life: changing your circle changes which rules work.

Worked example:

- The player has been among **Green** people — cooperative, trusting, consensus-seeking. A
  cooperative rule set works well and they reach the ceiling.
- They move among **Orange** people — competitive, self-interested, transactional.
- The same cooperative rules now get them **exploited**. Trusting by default is punished;
  generosity reads as weakness.
- They must build a second rule set: verify, negotiate, protect their position.

**Transfer failure runs both ways.** Neither rule set is correct — that is the entire point (§2):

| Imported rules | New environment | Failure mode |
| --- | --- | --- |
| cooperative, trusting | competitive, transactional | taken advantage of; generosity read as weakness |
| negotiate hard, optimise for self | cooperative, relational | frozen out; treated as untrustworthy |

**Daily conflict is the ongoing mechanic — DECIDED.** Living among a population means **resolving
conflicts with them every day**. That is the texture of an environment and the engine of the whole
loop.

**Generated from relationship state; resolved by rules or by hand.**

1. Each day the server generates friction that **follows from actual state** — who owes whom, who
   was slighted, what was left unresolved, who has been asked too often.
2. **Rules auto-resolve what they cover** (§3.14). The player reviews a log showing the situation,
   which rule fired, the action taken, and the consequence.
3. **Anything the rules don't cover escalates to the player** as a live decision.
4. So **good rules buy time, and gaps in the playbook are what demand attention.** That is the loop.

Repetition is what makes transfer failure legible: one bad outcome is noise, the same rule failing
daily for a week is a **pattern** — and a pattern is diagnosable (§3.6).

This unifies the design: the conflict-resolution challenge type is not one challenge — it is the
recurring daily engine of the environment system.

**It also implies two tiers of content**, which resolves part of the content-burn problem (technical.md §4):

| Tier | Length | Production |
| --- | --- | --- |
| **Authored challenges** (§3.10) | ~6 min | hand-built epics, 6–8 in V1 |
| **Daily conflicts** | short | **authored pathways per environment, selected from relationship state** |

Daily conflicts must stay light. If every day demanded a 6-minute epic the game would be
exhausting rather than habitual.

> **Daily conflicts are finite, and that is a real content cost.** The pool per environment has to
> be large enough that repetition reads as *the same kind of person doing the same kind of thing* —
> which is true to life and reinforces that a population has consistent values — rather than as the
> game running out. Pool size per environment is a scoping decision, not a tuning one.

Design consequences:

- **Build environments as populations**, not as places. A character set with shared values, reusing
  existing locations.
- **Duration is load-bearing.** A trip must last long enough — across real days — for failure to
  register as a *pattern* rather than one bad outcome. Don't allow a user to travel to new places twice within a single day. 
- **Both directions must be playable.** The lesson is never "Orange is bad"; it is that rules are
  local.
- **Characters must be consistent within an environment.** The population's values have to be
  legible purely through behaviour, never labelled (§3.2).

**V1 geography — DECIDED: same city, different home and different people.** The second environment
is a different home with a different population inside the existing city (§3.12). That is enough
to carry the thesis, since an environment is its people. **V2 adds genuinely different cities.**

**Open:** trip duration, and how many environments ship in V1.

### 3.16 Health and biomarkers — DECIDED

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

---

## 5. Scope — DECIDED

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
| Any runtime LLM — live AI interview, generated character dialogue, model-assisted challenge selection (§3.4, §3.6, §3.9) | V2+ |
| Genuinely different cities as environments — V1 uses one city, different home/people (§3.15) | V2 |
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

## 6. Open Requirements

### 6.1 Design questions still to answer

These need a decision before the systems they belong to can be built.

- **Relationship state model** — the per-character variables (trust, debt, grievance, warmth) that
  daily friction is generated from (§3.15). The largest of these, since the daily loop rests on it.
- **Trip duration and V1 environment count** — how long a stay lasts, and how many populations ship
  in V1 (§3.15).
- **Chat scoring** — what text feeds emotional stability, given the game must never appear to be
  gathering data about the person (§3.2, §3.5).
- **Organization routine** — named as a valuable habit to teach (§3.4) but still undefined as a
  mechanic.
- **Money: amount or source?** The curve (§3.5) is drawn against *amount held*, but the reasoning
  behind it was partly about *how* money is earned. Does earning method modulate the contribution,
  or only the balance?
- **Accounts** — anonymous instant play vs. login from day one (leaderboards and peer endorsement
  both imply identity).
- **City identity** — which city, real or fictional, and its district list (§3.12).
- **Fate of the Plan** — parked with onboarding (§3.13); the playbook (§3.14) now covers much of
  what it was for.
- **Onboarding proper** — deferred; V1 uses a random avatar (§3.13).

### 6.2 Confirm or cut — proposals awaiting a call

- Digital-surreal visual treatment reserved for moments of insight (§4, art bible).
- Imprecise biological age at low instrumentation, shown as a range (§3.16).
- Walk vs. transit as a real trade — encounters and activity load against money (§3.12).
