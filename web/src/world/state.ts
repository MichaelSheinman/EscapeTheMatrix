/*
 * The client's world model, and the only functions allowed to change it.
 *
 * technical.md §10.2: one normalised world-model that REST hydrates at boot,
 * socket messages patch, React subscribes to, and the renderer reads directly
 * in its own frame loop. This file is the model and the patches; `store.ts` is
 * the one instance of it, `react.ts` is how components read it, and
 * `../render/` is how the canvas does.
 *
 * Two rules from §10.2 shape everything below.
 *
 * 1. Patches are typed domain messages, not generic diffs. One reducer per
 *    message type, in a table the type system checks for completeness — a
 *    message added on the server becomes a compile error here rather than one
 *    silently ignored. A generic diff would make every socket message an
 *    accident instead of a decision.
 *
 * 2. The client computes no game rules. Every number in here was stated by
 *    the server. Nothing is derived, predicted, or incremented locally — which
 *    is what keeps `probes.count` honest when a command's response and a
 *    socket message disagree about it: the newer statement simply wins, and
 *    there is no local arithmetic to have drifted in between.
 *
 * The reducers are pure and take no dependency on the store, which is what
 * makes them the part worth unit-testing (`state.test.ts`).
 */

import type {
  BoardMoved,
  CareerChanged,
  ChallengeMoved,
  CharacterMoved,
  EventOffered,
  FactorStood,
  HealthBought,
  PlaybookComposed,
  RoutineSet,
  ServerMessage,
  ConflictAnswered,
  SituationAnswered,
  SocialOffered,
} from '../realtime/messages.gen'
import type { ProbeResult } from '../commands'
// `Tier` is taken in this file — instrumentation has tiers too (§3.16) — so
// §10.4's is imported under the name the world model calls it by.
import { FLOOR, type Tier as EffectsTier } from '../render/quality'
import type { Player } from '../session'

/** Who the server says we are (§9). Unknown until it has answered once. */
export type Session =
  | { status: 'unknown' }
  | { status: 'out'; notice: string | null }
  | { status: 'in'; player: Player }

/** What the push channel is doing (§8), as something the screen can say. */
export type Link = 'connecting' | 'live' | 'offline'

/**
 * What the renderer is doing.
 *
 * WebGL2 is a hard floor (§10.4): Pixi v8 removed the Canvas2D fallback, so a
 * device without it does not degrade, it fails. That is a state the world model
 * carries rather than an exception thrown into the void, because the only
 * useful response to it is a sentence on screen.
 */
export type Renderer = 'starting' | 'running' | 'unsupported'

/**
 * How the world is being drawn, and how well (§10.4).
 *
 * The effects tier is the one thing in this model that is a fact about the
 * *device* rather than about the world — like `renderer` above, and for the same
 * reason: the only useful response to it is a sentence and a button on screen,
 * so it has to be somewhere both React and the renderer can read. It is
 * remembered in this browser rather than on the server, because it is an
 * answer about this machine and a player's phone is not their desktop.
 */
export type Quality = {
  /** Which of §10.4's two tiers is being drawn. */
  tier: EffectsTier
  /** Whether the player asked for this, or it is simply the floor. */
  chosen: boolean
  /**
   * The world tried the High tier here and could not hold it.
   *
   * Set by the renderer's own frame clock, not by a guess about the hardware.
   * Cleared the moment the player asks for it again — a machine is allowed to
   * be told twice, and the alternative is a button that stops working.
   */
  demoted: boolean
  /** Frames a second, as the renderer last measured. 0 until it has. */
  fps: number
}

/**
 * One probe this client has been told about, and how it heard.
 *
 * `by` is the difference §8's rule 3 draws: a command's response tells you
 * about your own action, and the socket tells you about the world's. Keeping it
 * means the renderer can draw the two differently, which is the whole visible
 * proof that both paths land in the same store.
 */
export type Mark = {
  /** Which one this was, counting from the moment this client loaded. */
  n: number
  label: string
  notedAt: string
  by: 'you' | 'elsewhere'
}

/**
 * The walking skeleton's domain, and the shape every later module copies: a
 * number the server stated, and whatever presentation memory hangs off it.
 */
export type Probes = {
  /** Rows in `skeleton_probe`, as the server last said. Never computed here. */
  count: number
  /** How many this client has learned about since it loaded. */
  seen: number
  /** The most recent of those, oldest first. Bounded — see `REMEMBERED`. */
  marks: Mark[]
}

/**
 * One step out of a place: where it goes, how, and what it costs (§3.12).
 *
 * `mode` is the trade. `foot` is free and is where things happen to you;
 * `transit` takes the fare and passes through nothing. Neither costs game time
 * (§3.1), which is why there is no duration here to compare them by.
 */
export type Way = {
  to: string
  mode: 'foot' | 'transit'
  /** The step as a sentence fragment — what taking it reads as on a button. */
  says: string
  /** Cents, like every other amount. Zero for every walk. */
  fare: number
}

/**
 * Somewhere in the city. Authored vocabulary, sent by the server (§17).
 *
 * `layer` is §3.12's — a street, a building you can go into, or the home you
 * live in. The city map is not one of them: it is the district layer seen from
 * above, a way of choosing where to go rather than somewhere to go, and making
 * it a place would have made it a free ride between districts.
 *
 * `ways` travels with the place because a portal drawn in a painting is only
 * the *placement* of a way (§10.3) — most of the city is unpainted (#35), and an
 * unpainted place still has to be able to say how to leave it.
 */
export type Place = {
  key: string
  name: string
  note: string
  layer: 'district' | 'interior' | 'home'
  /** The district this place sits inside. Empty for a district. */
  in: string
  /** Whether the transit network calls here. Only streets have stops. */
  stop: boolean
  ways: Way[]
}

/**
 * Where in the painted location the player is standing (§10.3).
 *
 * Not a thing the server said, and the one slice of the model that is not.
 * That is deliberate and narrow: a painted location is authored data the client
 * loads directly (§10.3), and which authored rectangle a walk ended inside is a
 * fact about that data — geometry, not a rule. §10.2's "the client computes no
 * game rules" is untouched, because nothing here decides what *happens* in a
 * zone. Deciding that is the event system's (#21, #22), and the keys below are
 * the vocabulary it will be asked in: §3.9's placement conditions and a
 * physical spot in the world are the same object.
 *
 * Null when the world is not drawing a location — before boot, or in a district
 * nobody has painted yet.
 */
export type Standing = {
  /** The location's own key, as its map file names itself. */
  level: string
  /** The trigger zones containing the player, most specific first. */
  zones: { key: string; name: string }[]
  /**
   * The ways out this spot offers, as drawn.
   *
   * A portal is where a way *is* — walking onto one takes it (§3.12). Which
   * ways exist is the server's, and these are the authored places they are
   * stood on; a portal leading somewhere the city does not connect is refused
   * by the command rather than by the renderer.
   */
  portals: { key: string; name: string; to: string }[]
}

/**
 * The journey the player last made, as the server described it.
 *
 * Presentation memory of something that has already happened: where it went,
 * what the fare was, and how many places it passed through where something
 * could have happened to them. `chances` is zero for a ride, always — that
 * difference is §3.12's trade, and it is the part a player can see before the
 * event backlog (#21) turns a chance into an event.
 */
export type Trip = {
  by: 'foot' | 'transit'
  steps: { to: string; mode: 'foot' | 'transit'; says: string }[]
  fare: number
  chances: number
}

/**
 * The player: money, where they are standing, and where they live (§3.3).
 *
 * `money` is cents all the way to the screen — formatting it is presentation,
 * and a float on the wire arrives rounded. `earningsLeft` is derived by the
 * server on read (§3.4's cap against the player's own day), so it is a number
 * this client was told rather than one it worked out.
 */
export type Character = {
  money: number
  at: Place
  home: Place
  /**
   * The IANA timezone the character's day is measured in (§3.3). The server's
   * copy, not the device's: it is the zone the rollover runs against, and the
   * one the day/night phase below is read in.
   */
  timezone: string
  earningsTaken: number
  earningsLeft: number
}

/**
 * Whether it is day or night where the character is.
 *
 * §3.1: time of day is real. Night runs from 8 PM to 6 AM on the character's
 * own clock, and the world is drawn dark for it. Computed client-side on a
 * slow tick (`clock.ts`) — technical.md §3.3 rejects a server tick, and the
 * boundary is a fact about a clock everybody already holds.
 *
 * Named `phase` rather than anything with "night" in it because `nights` is
 * taken: a role's `nights: true` means the job is a night shift (§3.14).
 */
export type Phase = 'day' | 'night'

/** One level of instrumentation: what the player has bought the ability to see. */
export type Tier = {
  key: string
  name: string
  buys: string
  price: number
  owned: boolean
  forSale: boolean
}

/**
 * One readout on the health panel (§3.16).
 *
 * A metric the player has no instrument for still arrives — named, explained,
 * and carrying the tier that would read it — with `value` null and no history.
 * That is the design rather than an omission: what is withheld is the reading,
 * not the idea that the measurement exists.
 */
export type Metric = {
  key: string
  name: string
  unit: string
  means: string
  tier: string
  tierName: string
  lower: boolean
  decimals: number
  visible: boolean
  value: number | null
  history: number[]
}

/** The body, and what this player can see of it. */
export type Health = {
  /** Visible at every tier, composed by the server on read (§3.5, §3.3). */
  biologicalAge: number
  livedToday: boolean
  tiers: Tier[]
  metrics: Metric[]
}

/**
 * One answer a routine will take (§3.4).
 *
 * `says` is the rule as a sentence, because a routine *is* a rule and the
 * playbook composes in sentences too (§3.14) — one system, one interface
 * language. `costs` is the honest half: nothing in this game is labelled good or
 * bad (§3.2), so the price is stated and the verdict is the player's.
 */
export type RoutineOption = {
  key: string
  says: string
  costs: string
  chosen: boolean
}

/**
 * One routine: a standing question about the body, and the answers it will take.
 *
 * Every routine is answerable by everybody. There was a gate here — §3.6 held a
 * routine back until the player had named the problem it solved — and it is
 * gone, along with the `unlocked` flag and the sentence that said what would
 * open one. Nothing on this type is conditional on what the player has been
 * through.
 */
export type Routine = {
  key: string
  name: string
  asks: string
  chosen: string
  options: RoutineOption[]
}

/** The rules the player has set for their own body, as one unit. */
export type Routines = {
  /** Whether they have ever set them. Not the same as having chosen defaults. */
  set: boolean
  routines: Routine[]
}

/** One metric's arc under the rules currently in force. */
export type ForecastMetric = {
  key: string
  /** Empty for a metric this player has no instrument for — the panel's rule. */
  values: number[]
}

/**
 * What the routines would do over the next fortnight, if they were kept.
 *
 * Composed by the server on read and never stored (§3.3), and instrumented
 * exactly like the panel: a projection is a reading of a body too. Biological
 * age is the exception at both ends, visible at every tier.
 */
export type Forecast = {
  days: number
  biologicalAge: number[]
  metrics: ForecastMetric[]
}

/**
 * One number a parameterised playbook term may hold (§3.14).
 *
 * A closed authored list, never a range and never a free field: the interesting
 * question is *which* threshold, and a slider would only let a player tune one
 * until it fitted last week. The amount behind `says` stays on the server —
 * this client is handed the words and no arithmetic to do with them.
 */
export type PlaybookValue = {
  key: string
  says: string
}

/**
 * One word of the playbook's vocabulary: a condition a rule may test, or an
 * action it may take.
 *
 * `says` keeps its `{value}` placeholder for a term that holds a number, because
 * the builder puts the menu *inside* the sentence — "they are paying me [above
 * $60]" — and a finished string would have to be taken apart again to do that.
 *
 * A term this player has not unlocked is not here. Not blanked, not greyed,
 * not counted: the server never sent it. There is nothing in this model to
 * reveal by changing a component (§3.6).
 */
export type PlaybookTerm = {
  key: string
  says: string
  /** Empty for a term that takes no number. */
  values: PlaybookValue[]
}

/**
 * One standing question, and the whole vocabulary this player may answer it
 * from (§3.14).
 *
 * The pools belong to the question rather than to one global list: "they are
 * paying me above $60" is not a thing you can say about somebody raising their
 * voice at you. Questions arrive in authored order; the list's order is the
 * sequence.
 */
export type PlaybookQuestion = {
  key: string
  ask: string
  note: string
  about: string
  conditions: PlaybookTerm[]
  actions: PlaybookTerm[]
}

/** One part of a rule's sentence, or null for a part still blank. */
export type PlaybookRef = {
  term: string
  /** Empty for a term that takes no number. */
  value: string
}

/**
 * One question's answer inside one rule set.
 *
 * `when` holds one or two entries and either may be null. Two entries with the
 * second null is a player mid-edit — a different state from one entry, which is
 * a complete sentence — which is why the count is part of what is stored rather
 * than inferred from whether the second slot is filled.
 *
 * `inForce` is the server's statement, not a check to repeat here. Nothing
 * partial ever runs (§3.14), and a rule with a hole in it goes on escalating to
 * the player.
 */
export type PlaybookRule = {
  question: string
  when: (PlaybookRef | null)[]
  join: string
  then: PlaybookRef | null
  otherwise: PlaybookRef | null
  inForce: boolean
}

/**
 * Two of your answers that could disagree about the same moment (§3.14).
 *
 * Worked out by the server with no moment in hand — technical.md §6's fourth
 * argument for composing rules rather than writing them — and sent beside the
 * rules rather than fetched, because it is a statement about the set on screen
 * and it changes the instant a word does.
 *
 * `says` is written by the server. A client that assembled this sentence would
 * be deciding what a conflict means, and the first time the two disagreed the
 * player would be reading something the matcher never did.
 */
export type PlaybookConflict = {
  /** The two question keys, in authored order. */
  questions: string[]
  /** Disagrees whichever way each of them falls, rather than only sometimes. */
  always: boolean
  says: string
}

/** One environment's rules — §3.15's context-tagged rule set. */
export type PlaybookSet = {
  /** A place key, which `places` already has a name for. `name` is sent too. */
  environment: string
  name: string
  rules: PlaybookRule[]
  /** What this set says about itself with no moment in hand. Usually empty. */
  conflicts: PlaybookConflict[]
}

/**
 * The rules the player has composed for their own judgement (§3.14).
 *
 * Not one list. §3.15 makes it a set of context-tagged sets, one per
 * environment, because the whole thesis is that the right rules depend on where
 * you are — and there is one for every environment from the start, so arriving
 * somewhere new means finding an empty set rather than discovering the feature.
 *
 * There is no draft here beside a committed copy, and no save: editing the
 * sentence is the change. What replaces a save button is the blank.
 */
export type Playbook = {
  questions: PlaybookQuestion[]
  sets: PlaybookSet[]
}

/**
 * One thing the player's own history says (§3.6).
 *
 * A reading, with their own numbers in it, produced by the module that owns the
 * rows. Nothing here was authored for them to choose between — that is the
 * whole of §3.6's first rule, and the reason this type has no `chosen` field
 * and no order the server put it in.
 */
export type Observation = {
  key: string
  says: string
  /** Which log or instrument it was read off. */
  from: string
}

/**
 * One question a moment put, and what the playbook made of it (§3.14).
 *
 * `met` is per condition, in the sentence's own order — §3.14's "the player can
 * see exactly which condition matched", arriving as a field rather than as a
 * claim. `says` and `did` are rendered by the server for the same reason the
 * conflict sentence is: a screen that reassembled a rule out of its parts would
 * be a second implementation of what a rule means.
 */
export type Considered = {
  question: string
  /** `unanswered` | `undecided` | `weighed` | `fired`. */
  state: string
  says: string
  did: string
  /** `met` | `unmet` | `` (the moment never spoke to it), per condition. */
  met: string[]
  /** The whole `when`, combined by the join. */
  held: string
}

/** One moment the world put to the rules while the player was elsewhere. */
export type Fire = {
  id: number
  at: string
  environment: string
  situation: string
  /** `answered` | `silent` | `clash`. */
  verdict: string
  /** `good` | `poor` | `pending`. */
  outcome: string
  /** Why the moment escalated, in words, and empty when it did not. */
  note: string
  considered: Considered[]
  /**
   * The moment as it was *written*, for a record that came from a job (§3.14).
   *
   * Empty for every other reason a rule was ever weighed. The server joins them
   * on: internal/playbook holds which rule fired and internal/situation holds
   * what was happening, and neither may name the other.
   */
  name: string
  line: string
  /** What it came to, and empty while nothing has answered it. */
  came: string
}

/**
 * How one answer has been going over the window — §3.6's pattern.
 *
 * Counts rather than a grade, deliberately: "one bad outcome is noise, the same
 * rule failing daily for a week is a pattern", and a server that had already
 * concluded something from the counts would be doing the diagnosis the player is
 * supposed to do.
 */
export type PlaybookTrend = {
  question: string
  put: number
  fired: number
  weighed: number
  unanswered: number
  undecided: number
  good: number
  poor: number
}

/**
 * What the rules did, and what a week of it adds up to (§3.14, §3.6).
 *
 * Fetched rather than hydrated, and *not* part of the world model — history is
 * unbounded and a world model is not (§10.2). It belongs to whatever is looking
 * at it, exactly like a page of a leaderboard, and it goes away when that closes.
 */
export type PlaybookLog = {
  fires: Fire[]
  page: number
  pages: number
  total: number
  size: number
  /** The start of the window the trends cover, so a screen need not assume. */
  since: string
  trends: PlaybookTrend[]
}

/**
 * One place on the progression board (§3.5).
 *
 * A slot the player is not shown arrives with `shown: false` and *nothing else*
 * — no key, no name, no value. That is not this client withholding anything: the
 * fields are empty on the wire, because the server never put a hidden factor in
 * them. There is nothing here to reveal by changing a component.
 *
 * `reads` says how to render the number for the two that are shown, so this
 * client formats without knowing what either factor means.
 */
export type Slot = {
  key: string
  name: string
  reads: '' | 'age' | 'money'
  note: string
  /** Null for a slot the player is not shown. Not zero — a zero is a reading. */
  value: number | null
  shown: boolean
}

/**
 * Progression: the composite, and the board it was made of (§3.5).
 *
 * There is no level in this game. `karma` is what replaces it, composed by the
 * server on read from a body, a balance and three totals this client will never
 * see (§3.3) — so it is a number that was stated, like every other one here, and
 * it can fall.
 *
 * No maximum accompanies it, and none should be invented: a number drawn against
 * a ceiling is a progress bar however it is styled, and the whole point of §3.5
 * is that this is not one.
 *
 * `where`, `among`, `days` and `familiarity` describe the *environment* rather
 * than the player — which population they live among, how worn it is, and
 * therefore how much of an offered reward it still pays (§3.15). Safe to show,
 * and meant to be: a plateau nobody can feel is a plateau they grind against
 * instead of leaving.
 */
export type Karma = {
  karma: number
  slots: Slot[]
  /**
   * Every day behind this player and what the composite read at the end of
   * each, oldest first, with today on the end. The server keeps it, because a
   * finished day cannot be recomposed once the body it was a reading of has
   * moved on — this client stores what it is sent and draws it.
   *
   * One point on the day somebody joined. Not empty, and never plotted against
   * a maximum: see above.
   */
  curve: Point[]
  /**
   * The environment key: a *population*, not a place, so `places` has no name
   * for it and none should be looked for. It is also the tag on the rule set in
   * force (§3.15), which is the second and larger reason it is on the wire — the
   * server decides which set is running, because which people you are among is
   * a fact about where you live rather than about where you are standing.
   */
  where: string
  /** What that population is called. Empty where there is nobody. */
  among: string
  days: number
  familiarity: number
}

/** One day of the curve: the player's own local date, and what Karma read. */
export type Point = {
  day: string
  karma: number
}

/**
 * One board in §3.5's distributed leaderboard.
 *
 * There is one for the composite and one for every factor, including the
 * three that are never named — because a ranking is a position rather than a
 * number, and showing one discloses nothing about the formula. A sealed board
 * arrives with a key that names a slot (`slot-3`), no name, and no `reads`,
 * exactly as a sealed `Slot` arrives with nothing in it.
 */
export type BoardId = {
  key: string
  name: string
  note: string
  reads: '' | 'karma' | 'age' | 'money'
  shown: boolean
}

/** Where one player stands on one board. `rank` is 0 for somebody unplaced. */
export type Placing = {
  board: BoardId
  rank: number
  of: number
  /** Null on a sealed board, and on a board this player is not ranked on. */
  value: number | null
}

/**
 * Every board, with the reader's own standing on each (§3.5).
 *
 * Not part of the world snapshot. §10.2 names leaderboards as the one genuinely
 * *pull*-shaped surface in this client — a read-only paginated list of other
 * people — so it is fetched when somebody looks at it, and null until then.
 *
 * `refreshedAt` is the moment the server last rebuilt the boards, empty before
 * it ever has. A leaderboard is a snapshot by design (§3.4 puts the rebuild on
 * the job queue), and a number that is minutes old should say so.
 */
export type Boards = {
  refreshedAt: string
  players: number
  boards: Placing[]
}

/** One rung on a board: who, where, and — on a board that shows one — what. */
export type BoardRow = {
  rank: number
  name: string
  avatar: string
  /** Null on a sealed board. The server sent no number, so there is none. */
  value: number | null
  you: boolean
}

/**
 * One page of one board, as `GET /api/boards/{key}` answers it.
 *
 * Deliberately *not* in the world model. It is a page of other people, held by
 * whatever is looking at it and thrown away when that closes — the model is the
 * player's world, and a hundred strangers' ranks are not part of it.
 */
export type BoardPage = {
  board: BoardId
  rows: BoardRow[]
  page: number
  pages: number
  total: number
  size: number
  you: Placing | null
  refreshedAt: string
}

/** Another player, as any social surface is allowed to describe them. */
export type Person = {
  name: string
  avatar: string
}

/**
 * Somebody the game has offered this player to like, and until when (§3.5).
 *
 * `id` is the whole of the addressing. There is no account id anywhere in this
 * client — the leaderboards deliberately never send one and endorsement does not
 * need one, because an answer names the *offer* and the server already knows who
 * it named.
 */
export type Offer = {
  id: number
  who: Person
  /** RFC3339. When it lapses, which the screen says rather than the button
   *  quietly stopping working. */
  until: string
}

/**
 * Peer endorsement: the one surface in V1 where another human being moves
 * something (§3.5).
 *
 * `offer` is null most of the time, and that is the design — "rate it, don't
 * make it always-available" — rather than an empty state to apologise for. Which
 * is why `opens` sits beside it: a button that vanishes without saying when it
 * comes back is indistinguishable from a broken screen.
 *
 * The bottom half is what has been done *to* this player: how many people have
 * said they like them, and the most recent few by name. It is a count of people
 * and nothing else — nothing here says what being liked is worth, or that it is
 * worth anything at all.
 */
export type Endorsement = {
  offer: Offer | null
  given: number
  cap: number
  /** RFC3339, or empty when an offer is standing or one may be drawn now. */
  opens: string
  received: number
  liked: Person[]
}

/**
 * One thing the world has put in front of the player, right now (§4).
 *
 * `place` and `zone` are where it is, by key. The zone is the named trigger
 * zone a painting draws (§10.3) — the same object the event's placement rules
 * are written in — so a level that has such a spot can pin a marker in it, and
 * one that has not simply does not. Neither the client nor the painting decides
 * whether it *may* be there: the server placed it before it said a word.
 *
 * `accept` and `decline` are the two answers in the content's own words, sent
 * as text rather than as keys to look up. The catalogue is content in the repo
 * (§5.2), and a client holding its own copy would show yesterday's writing
 * after a deploy — the same argument the city travels under.
 */
export type EventOffer = {
  id: number
  name: string
  /** interpersonal | work | money | opportunity. */
  kind: string
  line: string
  accept: string
  decline: string
  /**
   * Why this reached you, for an escalation: which moment it was, who it was
   * with, and which of your own rules fired in it. Empty for every ordinary
   * event — a knock at the door has no why beyond itself.
   */
  because: string
  /** The moment it is about, in full — null for every ordinary event. */
  about: EventAbout | null
  place: string
  zone: string
  /**
   * What the world draws where this is, by name — absent for the ordinary
   * event, which gets whatever a surface pins on an event.
   *
   * A name and never a picture (§10.3 keeps the drawing on the client side), so
   * what the server is choosing is *which* of the shapes this build already
   * knows how to draw. `fire` is the one that exists: the renderer puts a
   * building alight at `zone`, and the map marks the place it is in.
   */
  glyph?: string
  /** RFC3339. When it lapses, which the screen says out loud. */
  until: string
}

/**
 * The event surface (§4): the one thing standing in front of the player, or
 * nothing.
 *
 * One field, and the absence of a second one is the design. The backlog behind
 * this — how much the world has ready, and what — is deliberately not sent: a
 * client that could read it could walk towards it, and §4's sandbox depends on
 * not being a quest list. `offer` is null most of the time and that is a world
 * going about its business rather than an empty state to apologise for.
 */
export type Events = {
  offer: EventOffer | null
}

/**
 * One thing that happened, as `GET /api/events/log` reads it back.
 *
 * Deliberately *not* a slice of the world model, like `BoardPage` and the
 * playbook's log: history is unbounded and a world model is not (§10.2). It is
 * held by whatever is looking at it.
 */
export type EventRecord = {
  id: number
  name: string
  kind: string
  at: string
  place: string
  zone: string
  /**
   * accepted | declined | left | lapsed | dropped.
   *
   * The last three are all "not answered" and are three words rather than one:
   * walking away, not looking, and the world giving up are different things to
   * be told about your own week.
   */
  answer: string
  /** RFC3339, or empty while an accepted event is still running. */
  resolved: string
  /**
   * The moment as it was put to you, the choice in its own words, and what it
   * came to. The server's writing, resolved from the catalogue when the record
   * is read — so an event whose content was edited reads back edited, and one
   * withdrawn since reads back as a title and a place with nothing under it.
   *
   * `did` is empty for the three answers that were never a choice, because
   * walking past something is not picking anything. `came` is empty for
   * everything that was not accepted and finished on the spot.
   */
  line: string
  did: string
  came: string
  /**
   * Why an escalation reached you — see `EventOffer.because`. Stored on the
   * record rather than resolved, so a week-old escalation still reads back as
   * the moment it was actually about.
   */
  because: string
  /** The moment it was about, in full — null for every ordinary event. */
  about: EventAbout | null
}

/**
 * What an escalation is about: the moment itself, and where its two halves
 * live.
 *
 * The three escalation events are the only ones in the game that are about
 * something that already happened — a conversation with somebody, answered by a
 * rule the player wrote. `says`, `did` and `came` are that exchange in the
 * order it happened: what they said, what your character said back, and what
 * they sent afterwards. The last of those is the consequence, in the authored
 * words it happened in, and it is the only thing this screen ever offers as an
 * account of how it went — nothing here grades it.
 *
 * `environment`, `person` and `message` are the conversation on the phone, and
 * each rule carries the set and question it lives under in the playbook. They
 * are keys because they are somewhere to *go*: reading what happened and being
 * unable to reach either half of it is what this data exists to fix.
 */
export type EventAbout = {
  name: string
  says: string
  did: string
  came: string
  /** Who it was with, and empty for a moment that was with nobody. */
  who: string
  /** The thread it is part of, all empty for a moment not on the phone. */
  environment: string
  person: string
  message: number
  /** Every answer weighed in the moment, in the order the questions were put. */
  rules: EventWeighed[]
}

/** One of your own answers as it stood in that moment, and where to find it. */
export type EventWeighed = {
  environment: string
  question: string
  /** The rule as one sentence, and the action it came down to. */
  says: string
  did: string
  /** fired | weighed | undecided — internal/playbook's word, not a verdict. */
  state: string
}

export type EventLog = {
  records: EventRecord[]
  page: number
  pages: number
  total: number
  size: number
}

/**
 * One dealing with one person, as a thread reads it back (§3.15).
 *
 * Three parts of one exchange: `says` is what they did, `did` is what your
 * character did about it, and `came` is what followed. All three are the
 * server's own writing — the same rule the conflict surface and the playbook's
 * log follow, because a client that assembled a moment out of its parts would
 * be a second implementation of what the moment means (§10.2).
 *
 * `covered` is the one the screen is read for: a message your rules answered is
 * one your character replied to while you were somewhere else.
 */
export type PhoneMessage = {
  id: number
  at: string
  on: string
  name: string
  /** One string per chat bubble, in the order they arrive. */
  says: string[]
  /** Empty exactly while this is the message waiting on you. */
  did: string
  /** One string per chat bubble, the same as `says`. */
  came: string[]
  covered: boolean
  /** good | mixed | poor, and pending while it waits. */
  outcome: string
  waiting: boolean
  /** What may be said back. Empty for everything that is not waiting. */
  answers: SituationAnswer[]
}

/**
 * One person and everything that has passed between you.
 *
 * Note what is not on it: any sentence about what this person is like. The
 * population is legible through behaviour and never labelled (§3.15), and a
 * thread is the whole of the evidence — six messages from somebody is how a
 * player works out what they are, and one line describing them is the game
 * doing it for them.
 */
export type PhoneThread = {
  key: string
  name: string
  where: string
  environment: string
  /** The population as a person reads it — the name a quiet thread is named by. */
  among: string
  /** Whether they are still somebody you live among. */
  here: boolean
  /**
   * How the two of you met, ahead of every message. `them` is who is speaking;
   * false is the player. Carries no clock or day — it is the same text for
   * every player rather than something that happened on a date.
   */
  intro: { them: boolean; says: string }[]
  /** Oldest first, which is the order a conversation is read in. */
  messages: PhoneMessage[]
  waiting: boolean
  last: string
}

/**
 * The phone, fetched rather than hydrated.
 *
 * Unpaged and bounded on the server by how far back it reads, because threads
 * are bounded by the population — six people, not six hundred — and a
 * conversation read backwards a page at a time is not a conversation.
 */
export type Phone = {
  threads: PhoneThread[]
}

/**
 * A challenge standing where the player is (§4's availability half).
 *
 * A name and a place, and nothing about what it is. The scenes, the options and
 * how long it takes are all on the other side of beginning it: a client holding
 * the content would show yesterday's writing after a deploy, and one that could
 * read the graph before entering would turn §3.10's committed judgement into a
 * preview of where each reading leads.
 */
export type ChallengeDoor = {
  /** The code, and what the begin command names. */
  challenge: string
  name: string
  place: string
  /** The trigger zone a painting draws the marker in (§10.3). */
  zone: string
  /**
   * What this says to somebody standing at it, and the label on the button
   * that begins it. Authored per challenge — every word inside a challenge is
   * content, and this screen writes none of its own.
   */
  door: string
  enter: string
}

/** One line of a scene. `who` is empty for the narration itself. */
export type ChallengeBeat = { who: string; line: string }

/**
 * One authored position, in the words the player reads (§3.10).
 *
 * None of them is correct and none of them is marked. What an option is worth
 * is authored on the server and paid on completion (§5.1) — it is not here,
 * it is not derivable from here, and that is the hidden scoring holding.
 */
export type ChallengeChoice = { key: string; text: string }

/**
 * One passage through one challenge, as the server last stated it.
 *
 * Everything needed to re-establish the moment, because §5.1 makes save and
 * resume mandatory: this is what a returning player lands on rather than a list
 * they choose from again. `scene` is the checkpoint's key and is what the choose
 * command names, so a tab answering a question the run has already left is
 * refused rather than applied twice.
 */
export type ChallengeRun = {
  challenge: string
  name: string
  scene: string
  /** Where the run is standing, in the content's own words. */
  where: string
  /** How the last thing committed to was received — not what it meant (§3.10). */
  taken: string
  beats: ChallengeBeat[]
  ask: string
  choices: ChallengeChoice[]
  /** How far through, as a length rather than a mark. */
  stage: number
  stages: number
  /**
   * Which mechanic type is running this, so the shell can pick the scene module
   * built for it (§5.2). `dialogue_branch` is people talking; `salvage` is a
   * room with more in it than you can carry.
   */
  mechanic: string
  /**
   * The painted location this run stands in, or absent.
   *
   * A location key rather than a city place: the burning flat is somewhere a
   * run puts you and never somewhere you can walk to. The renderer shows it for
   * the length of the run; a build with no such painting keeps showing the
   * world the character is actually standing in.
   */
  canvas?: string
  /**
   * What is in the bag, in the content's own words, and how much it holds.
   *
   * Absent for a mechanic with no notion of carrying anything, which is most of
   * them. Sent as a list rather than as a sentence in the beats because the
   * screen draws slots filling up, and a client that had to parse prose to do
   * that would be a client that breaks when somebody edits the prose.
   */
  held?: { key: string; name: string }[]
  holds?: number
  done: boolean
  /** How much the composite moved. Negative when it fell; absent when it did not. */
  karma?: number
  /** Money the run put in the balance, in cents. Absent when none. */
  earned?: number
  /** The reaction authored for the outcome it landed in (§5.1). Empty until done. */
  closing: string
  began: string
  moved: string
}

/**
 * The challenge surface (§5): the run in progress, or a door into one.
 *
 * Never both. Somebody in the middle of a six-minute endeavour is not also
 * being shown the way into another one — the resume is the thing that has to be
 * unmissable, and an entry point beside it would be an invitation to abandon a
 * run by starting a second.
 */
export type Challenge = {
  here: ChallengeDoor | null
  run: ChallengeRun | null
}

/**
 * One thing the character could do about a moment their rules did not settle
 * (§3.14).
 *
 * `key` is what the answer command names. What is deliberately *not* here is
 * what it is worth: the authored deltas (§3.5) are the server's, exactly as a
 * challenge option's are, and a client that could see them would be reading a
 * grade off a menu.
 */
export type SituationAnswer = {
  key: string
  /** What your character would do, in the second person. */
  does: string
}

/**
 * A moment a delegated role produced that nobody's rules settled (§3.14).
 *
 * It is a live decision rather than a notification: the role ran while the
 * player was elsewhere, the playbook could not answer this one, and the answer
 * is theirs to give. `why` says which of §4's conditions brought it here and
 * never what to do about it.
 */
export type WaitingSituation = {
  id: number
  name: string
  line: string
  /** The job that produced it, in the words somebody in it would use. */
  role: string
  environment: string
  why: string
  /** The player's own day the role produced it on, and when it was written. */
  on: string
  at: string
  answers: SituationAnswer[]
}

/**
 * The delegated-work surface (§3.14): the one moment waiting, or nothing.
 *
 * Never more than one. A role that is waiting on the player produces nothing
 * new, which is what makes "good rules buy time, and gaps in the playbook are
 * what demand attention" a mechanic rather than a slogan — and what keeps a
 * fortnight away from being a fortnight of decisions (§4).
 */
export type Situations = {
  waiting: WaitingSituation | null
}

/**
 * One person you live among, and how it has been going (§3.15).
 *
 * This type has no disposition, no alignment and nothing that could be drawn as
 * a badge: "the population's values have to be legible purely through behaviour,
 * never labelled." A player works out what these people are like by reading the
 * conflicts they turn up in, which is the same way it works everywhere else.
 *
 * `standing` is what is outstanding between you, in the server's own sentences.
 * Never numbers — a client handed four integers would have to be told what they
 * mean, which is the same thing as holding a copy of the mechanic (§10.2).
 *
 * `lives` and `known` are the other kind of field entirely: an address and a
 * connection, both of them facts. They are what the phone's contacts card is
 * made of — a card that summarised somebody's behaviour would be the roster
 * panel §3.2 had removed.
 */
export type Neighbour = {
  key: string
  name: string
  note: string
  /** Where they live, and how the character knows them. Facts, not description. */
  lives: string
  known: string
  /** The city place they turn up in, by key. */
  where: string
  standing: string[]
  /** Conflicts inside the window, and how they went. */
  met: number
  good: number
  poor: number
}

/**
 * How one of the playbook's questions has been going among these people —
 * §3.6's pattern, counted.
 *
 * `covered` is the number the whole design turns on: "good rules buy time, and
 * the holes in your playbook are exactly what demands your attention." How many
 * of the fortnight's conflicts the player's own rules settled, against how many
 * came to them, is the one thing this surface exists to make readable.
 */
export type Recurring = {
  question: string
  /** The question in the player's own words. */
  ask: string
  met: number
  covered: number
  good: number
  poor: number
}

/** Who the player lives among, and what a fortnight of it came to (§3.15). */
export type Among = {
  environment: string
  name: string
  note: string
  /** What these people have in common, written as behaviour. */
  holds: string[]
  people: Neighbour[]
  questions: Recurring[]
  /** The first day the counts include. */
  since: string
}

/**
 * A conflict with somebody you live among that nobody's rules settled (§3.15).
 *
 * A live decision rather than a notification, exactly like a waiting situation —
 * and with two things a situation has not got: who it is with, and what is
 * between the two of you. Both are on the screen because §3.6 requires a player
 * to be able to work out why the world did something, and a conflict that
 * followed from a debt is only legible as following from one if the debt is
 * there to read.
 */
export type WaitingConflict = {
  id: number
  name: string
  line: string
  /** The person it is with. */
  who: string
  where: string
  environment: string
  why: string
  standing: string[]
  on: string
  at: string
  answers: SituationAnswer[]
}

/**
 * The daily-conflict surface (§3.15): the one thing waiting, and the people it
 * came from.
 *
 * Never more than one waiting. Nothing new comes up while the last thing is
 * unresolved, which is what makes "good rules buy time" a mechanic rather than
 * a slogan — and what keeps a fortnight away from being a fortnight of
 * decisions (§4).
 */
export type Conflicts = {
  waiting: WaitingConflict | null
  among: Among | null
}

/**
 * One job, in the words the player reads.
 *
 * `pay` is cents a day and stays cents — formatting is presentation, and a
 * float on the wire would arrive rounded. `hours` and `nights` are here because
 * they are the honest half of what a job costs: a posting showing its salary
 * and not its shift is making half the argument.
 *
 * There is no `tier` (2026-08-15). Jobs do not sit on a ladder any more — each
 * one states its own wage, its own shift and its own way in.
 */
export type Role = {
  key: string
  name: string
  /**
   * The job as it is advertised, in the sections a real posting has
   * (content/jobs/SPEC.md, 2026-08-09).
   *
   * Some of these jobs quietly cost their holder something, and nothing in
   * here says which: every posting carries the same three sections, so the
   * layout says nothing about which line matters — the judgement stays the
   * player's.
   */
  posting: { intro: string; duties: string[]; qualifications: string[] }
  pay: number
  hours: number
  nights: boolean
  /** The working day as the posting states it, "21:00–07:00", composed on the server. */
  shift: string
}

/**
 * One thing a job asks for, as the player is allowed to read it.
 *
 * `hidden` carries no label and no verdict. It is not a masked value: three of
 * the five factors are ones the player is never told exist (§3.5), so a blanked
 * chip that flipped to `met` would name what it was measuring at the moment it
 * stopped mattering. What they get is that something else is being asked.
 */
export type Want = { label: string; state: 'met' | 'unmet' | 'hidden' }

/**
 * One job for a number of days: an entry of a route, or what is still owed on
 * one. The name travels with the key so the client never holds a second copy of
 * the catalogue.
 */
export type Tenure = { role: string; name: string; days: number }

/**
 * One way into a job: the work it asks for, and what of that work is still
 * owed.
 *
 * `needs` is the route as authored; `short` is the same list with the days
 * already worked taken off, and only the entries still outstanding. A finished
 * route has an empty `short` and keeps its `needs` — "45 of 45 days" is worth
 * reading.
 */
export type Route = { needs: Tenure[]; short: Tenure[] }

/**
 * A job the player does not hold, and what stands between them and it.
 *
 * At most one way in is ever set. `walk` is a job taken on where they are
 * standing; `interview` is the authored challenge they have to get through
 * (§3.4), and it is only sent when they may actually sit it. Neither means a
 * job nothing hires into yet — those are on the board and out of reach.
 */
export type Opening = {
  role: Role
  wants: Want[]
  /** Whether they reach all of it — including the parts `wants` will not describe. */
  ready: boolean
  walk: { place: string; zone: string } | null
  /**
   * Whether the board takes them into it from right here — a walk-in role, or
   * an interview already passed. Separate from `ready` on purpose: this is
   * whether the door exists, that is whether they fit through it.
   */
  take: boolean
  /** The challenge that hires into it, sittable from anywhere — interviews happen online. */
  interview: string
  /**
   * Every route into this job, nearest first, and empty for a posting that asks
   * for no previous work (2026-08-15).
   *
   * Any one of them qualifies — they are alternatives, and the screen has to
   * say so. The order is the server's so two tabs rank them the same.
   */
  experience: Route[]
  /** Whether any route through it is complete. */
  worked: boolean
}

/**
 * The job surface: what is held, what is open, and all the work this population
 * has.
 *
 * The last is the piece worth arguing for. Work only teaches a trade if
 * somebody can see where it goes, so every posting arrives with its pay and its
 * shift — most of them things this player cannot have yet.
 *
 * There is no `next` (2026-08-15). There is no step up: there are postings,
 * each stating the work it asks for, and the player decides which one they are
 * walking towards.
 */
export type Career = {
  held: Role | null
  since: string
  /** What one day of it is worth, in cents. Zero for nobody. */
  paid: number
  /** A job being taken on where they are standing — the map's second way in. */
  here: Opening | null
  /**
   * The board (2026-08-08): every job with a way in, as a posting — read the
   * job, read the shift, and act on it from anywhere. The ones nothing hires
   * into are `work`'s to show, not the board's.
   */
  board: Opening[]
  /** Every job this population is offered, cheapest first. */
  work: Role[]
}

/** A metric that moved between named bands — the simulation, saying so. */
export type Crossing = {
  metric: string
  from: string
  to: string
  worse: boolean
}

/**
 * The last command this client sent, whatever became of it.
 *
 * In the world model rather than in a component, because the canvas sends the
 * same commands the panels do (§10.1) — a tap that failed has to be able to say
 * so on a surface the tap did not come from. Holding the key is also the whole
 * of what a client must do to make its retries safe (§8): the same key is the
 * same intent, and a new one is a new action.
 */
export type Attempt =
  | { status: 'idle' }
  | { status: 'sending'; key: string }
  | { status: 'applied'; key: string; replayed: boolean }
  | { status: 'refused'; key: string; code: number; message: string }

/**
 * Whether a command is on the wire right now.
 *
 * The client's one mutex: every action brackets its fetch with `attempt`, so
 * any surface with a button holds still while any other surface's command is
 * in flight. One selector rather than eight copies of the expression, because
 * a surface that wrote its own would be deciding what "busy" means.
 */
export const sending = (w: World): boolean => w.attempt.status === 'sending'

/**
 * What the server says it is running on.
 *
 * Null until the world has been hydrated once. Stated by the server rather than
 * detected here — like every other number on screen (§10.2).
 */
export type Server = { database: string; schema: number }

/**
 * Which way the player is asking to walk, in plate axes, unit length.
 *
 * A client-side fact like the effects tier: the renderer reads it every frame
 * and steers the character while it is held. Null when no key is down. It never
 * touches the server — moving about inside one location changes nothing the
 * server holds, exactly as a tap-to-walk does not (§10.1).
 */
export type Heading = { x: number; y: number }

/**
 * What the player wears, and nothing else about them (client-side, cosmetic).
 *
 * A closed vocabulary of authored swatches — never a free field. Null in a slot
 * is the none option, which is a choice rather than the absence of one. Kept in
 * this browser until the server grows an appearance model; the mirror and the
 * figure in the street both read it, which is what makes a pick instant.
 */
export type Look = {
  coat: string | null
  hair: string | null
  hat: string | null
  bag: string | null
}

export type World = {
  session: Session
  link: Link
  renderer: Renderer
  /** Which way the held arrow keys are steering the character, if any. */
  heading: Heading | null
  /** What the player wears — cosmetic, this browser's own (see `Look`). */
  look: Look
  /** Which effects tier the world draws at, and how it is coping (§10.4). */
  quality: Quality
  /** Day or night on the character's own clock (§3.1). See `Phase`. */
  phase: Phase
  attempt: Attempt
  server: Server | null
  /** Null until the world has been hydrated once. */
  character: Character | null
  /** The city — every place and every way between them. Empty until hydration. */
  places: Place[]
  /** The last journey made, whatever it was. Null until one has been. */
  trip: Trip | null
  /** Where in the painted location they are standing. Null until they are. */
  standing: Standing | null
  /** Null until the world has been hydrated once. */
  health: Health | null
  /** The rules set for the body, and where they lead. Null until hydration. */
  routines: Routines | null
  forecast: Forecast | null
  /**
   * The rules set for judgement, one set per environment (§3.14, §3.15). Null
   * until hydration.
   *
   * In the boot snapshot rather than fetched, unlike the boards: it is the
   * player's own world, and the screen that edits it has to open on what is
   * already in force rather than on a spinner.
   */
  playbook: Playbook | null
  /** Progression, as the server last composed it. Null until hydration. */
  karma: Karma | null
  /**
   * The leaderboards, once somebody has looked at them (§3.5).
   *
   * Null until then, and *not* restored by hydration — this is the one slice
   * the boot snapshot does not carry, because it is a read-only paginated list
   * rather than part of the world (§10.2).
   */
  boards: Boards | null
  /**
   * Peer endorsement (§3.5). Null until the world has been hydrated once.
   *
   * Unlike the boards, this *is* in the boot snapshot: an offer and a handful of
   * names are the player's own world, whereas a board is a page of other people
   * (§10.2). Reading the snapshot is also the only thing that ever draws an
   * offer, so hydration is not merely how the client learns about one — it is
   * how one comes to exist.
   */
  endorsement: Endorsement | null
  /**
   * What the world has put in front of the player (§4). Null until hydration.
   *
   * In the boot snapshot for the endorsement offer's reason: it is the player's
   * own world, and reading the snapshot is one of the two things that can cause
   * an offer to exist at all. What is *not* here is the backlog behind it.
   */
  events: Events | null
  /**
   * §5's challenges: the run this player is part way through, or a challenge
   * standing where they are. Null until hydration.
   *
   * In the boot snapshot because §5.1 makes save and resume mandatory — a
   * player who closed the tab in the middle of a conversation has to be put
   * back into it by the same read that establishes everything else, not by a
   * second fetch that might not happen.
   */
  challenge: Challenge | null
  /**
   * The job surface: what is held, what is open, and the work on offer. Null
   * until hydration.
   *
   * In the boot snapshot because it is the player's own standing arrangement
   * rather than something that happens to them — and because the forecast is
   * computed against the job the server says they hold, so a client showing one
   * without the other would be showing an arc and its cause apart.
   */
  career: Career | null
  /**
   * §3.14's delegated work: the one moment an active role produced that the
   * player's own rules could not settle. Null until hydration.
   *
   * In the boot snapshot because it is the player's own world — a decision
   * waiting on them, like a challenge run they are part way through. Unlike an
   * event, reading it cannot cause one: a situation is produced by a day of the
   * rollover passing and by nothing else.
   */
  situations: Situations | null
  /**
   * §3.15's engine: the people the player lives among, what a fortnight of
   * living beside them came to, and the one piece of friction their own rules
   * could not settle. Null until hydration.
   *
   * In the boot snapshot for the same reason the situations are: it is the
   * player's own world, and reading it cannot cause one — a conflict is
   * produced by a day of the rollover passing and by nothing else.
   */
  conflicts: Conflicts | null
  /**
   * What the last day lived moved across, newest first.
   *
   * Presentation memory of an effect the pure core reported (§3.1), kept so
   * the panel can say what changed rather than only what is.
   */
  crossings: Crossing[]
  probes: Probes
  /**
   * The developer's toolbar, when somebody has opened it.
   *
   * Null in every ordinary session and null in production always — the server
   * does not register the commands that fill it, so nothing ever writes here.
   * It is in the store rather than in the component's own `useState` because
   * enabling a challenge moves two *world* surfaces beside it, and the three
   * arriving from one response have to be applied as one patch.
   *
   * It is deliberately not part of the boot snapshot. Nothing about this is the
   * player's world, and hydrating it would put the catalogue on the wire for
   * everybody — which is the one thing §4 keeps off it.
   */
  dev: DevToolbar | null
  /**
   * The toolbar's conversation half, when somebody has opened it.
   *
   * Null under exactly the same conditions as `dev` above and for the same
   * reasons — the server does not register the commands that fill it outside a
   * development build, and none of it is the player's world.
   *
   * A slice of its own rather than more fields on `DevToolbar`, because the two
   * halves are filled by different commands and a response carrying one must
   * not blank the other: enabling a challenge would otherwise wipe the thread
   * somebody was three presses into assembling.
   */
  devTalk: DevConversations | null
}

/**
 * What the toolbar knows: the whole catalogue, and this player's state for each.
 *
 * The shape mirrors internal/api's `devResult` minus the two surfaces, which go
 * to their own slices. `notice` is a developer-facing sentence and is the one
 * string in this client that does not have to be in the world's voice.
 */
export type DevToolbar = {
  challenges: {
    challenge: string
    name: string
    mechanic: string
    gate: string
    opens: string
    places: string[]
    on: boolean
    played: boolean
    running: boolean
  }[]
  notice: string
}

/**
 * What the toolbar's conversation half knows.
 *
 * Mirrors internal/api's `devConversationResult` minus the conflict surface,
 * which goes to its own slice. The phone is not here: the panel listed the
 * threads itself for a while, which was a second rendering of what the phone
 * screen already draws properly, and the raise buttons are what the strip is
 * for. Press raise, then press T.
 *
 * `pressure` and `under` on a person are the numbers the server's own selection
 * sorted on. They are on screen so that "why did it pick her again" has an
 * answer that is not a guess.
 */
export type DevConversations = {
  pathways: {
    conflict: string
    among: string
    name: string
    follows: string
    puts: string
    line: string
    answers: string[]
    /** Who may send it, by person key — a pathway belongs to people now. */
    with: string[]
    /** What must already have passed with the same person, as an author reads it. */
    after: string[]
    /** Days before it comes round again, or 0 for one that is said once. */
    every: number
    here: boolean
  }[]
  people: {
    person: string
    name: string
    pressure: number
    under: string[]
    met: number
    today: boolean
    /**
     * Every pathway that could be put to this person right now, by key — theirs
     * to send, with whatever it waits on already behind them, and under a
     * pressure they are actually under. The toolbar draws its raise buttons
     * from this, so a moment out of turn is never offered rather than offered
     * and refused.
     */
    raise: string[]
  }[]
  where: string
  notice: string
}

/**
 * How many marks the client keeps.
 *
 * This is presentation memory, not state: the count above is the truth, and
 * these are the individual events behind the last stretch of it. Unbounded, a
 * long session would hand the renderer thousands of sprites describing
 * something the number already says.
 */
export const REMEMBERED = 48

export const initialWorld: World = {
  session: { status: 'unknown' },
  link: 'connecting',
  renderer: 'starting',
  heading: null,
  look: { coat: null, hair: null, hat: null, bag: null },
  // §10.4 ships Low first, and Low is what a device gets until somebody says
  // otherwise. Not a guess at the hardware: a guess would be wrong on exactly
  // the machines the tier exists for.
  quality: { tier: FLOOR, chosen: false, demoted: false, fps: 0 },
  // Day until the clock says otherwise, which it does before the first frame:
  // `startDayCycle` ticks once at boot, so a player opening the app at night
  // never sees the daylit world flash first.
  phase: 'day',
  attempt: { status: 'idle' },
  server: null,
  character: null,
  places: [],
  trip: null,
  standing: null,
  health: null,
  routines: null,
  forecast: null,
  playbook: null,
  karma: null,
  boards: null,
  endorsement: null,
  events: null,
  challenge: null,
  career: null,
  situations: null,
  conflicts: null,
  crossings: [],
  probes: { count: 0, seen: 0, marks: [] },
  dev: null,
  devTalk: null,
}

/**
 * What `GET /api/world` answers — the boot hydration of the model above (§10.2).
 *
 * Hand-written, unlike the socket's message types, which are generated from the
 * Go structs (§8, rule 1). Generation covers the push protocol because that is
 * where a silent mismatch turns into a message quietly ignored; this one is a
 * single read whose Go definition is `internal/api/world.go` and whose failure
 * mode is the whole screen being empty. Keep the two in step by hand.
 */
export type Snapshot = {
  character: Character
  places: Place[]
  health: Health
  routines: Routines
  forecast: Forecast
  playbook: Playbook
  karma: Karma
  endorsement: Endorsement
  events: Events
  challenge: Challenge
  career: Career
  situations: Situations
  conflicts: Conflicts
  probes: number
  database: string
  schema: number
}

/* ── patches ───────────────────────────────────────────────────────────────
 *
 * Every one of these is `(world, thing) => world`, and each keeps the *same*
 * object for the slices it did not change. That matters more here than in an
 * ordinary app: the renderer reads the store every frame and skips work by
 * comparing references, so a reducer that clones unconditionally would rebuild
 * the scene sixty times a second.
 *
 * Hydration is the one that restates whole slices rather than comparing them —
 * it is the server saying everything it knows, and it happens twice in a
 * session: at boot, and after a socket gap.
 */

/** Boot, and rehydration after the socket reports a gap it cannot replay. */
export function hydrated(world: World, snapshot: Snapshot): World {
  const server =
    world.server?.database === snapshot.database && world.server.schema === snapshot.schema
      ? world.server
      : { database: snapshot.database, schema: snapshot.schema }

  const probes =
    world.probes.count === snapshot.probes
      ? world.probes
      : { ...world.probes, count: snapshot.probes }

  // The character and the city are replaced wholesale rather than compared,
  // because a hydration is the server restating everything it knows and there
  // is nothing here a stale reference would save.
  return {
    ...world,
    server,
    probes,
    character: snapshot.character,
    places: snapshot.places,
    health: snapshot.health,
    routines: snapshot.routines,
    forecast: snapshot.forecast,
    playbook: snapshot.playbook,
    karma: snapshot.karma,
    endorsement: snapshot.endorsement,
    events: snapshot.events,
    challenge: challengeAfter(world.challenge, snapshot.challenge),
    career: snapshot.career,
    situations: snapshot.situations,
    conflicts: snapshot.conflicts,
  }
}

/**
 * What the challenge surface is after a hydration.
 *
 * The one place in this file where the client keeps something the server did
 * not just say, and it is presentation memory rather than a rule — the same
 * kind of memory `crossings` and the probe marks are. A run that has *finished*
 * is not part of the world any more, so the snapshot stops carrying it the
 * moment the last option is committed; what the player is reading at that
 * moment is the reaction authored for the outcome they reached, which is the
 * only account of it they will ever get (§5.1). Throwing that off the screen
 * because the world was re-read a second later would take back the ending of a
 * six-minute endeavour.
 *
 * Nothing is computed here. The finished run is a sentence the server wrote,
 * held until the server has something else to say about that surface.
 */
function challengeAfter(held: Challenge | null, said: Challenge): Challenge {
  if (held?.run?.done && !said.run && !said.here) return held
  return said
}

/**
 * A probe this tab just made, from the command's own response.
 *
 * §8, rule 3: the acting client applies the response directly and does not wait
 * for a socket echo — which is also why the server does not send it one. This
 * is that half, and `probe.recorded` below is the other.
 */
export function probed(world: World, result: ProbeResult): World {
  return {
    ...world,
    probes: remember(world.probes, result.probes, {
      label: result.label,
      notedAt: result.notedAt,
      by: 'you',
    }),
  }
}

/** The character as a command that changed it reported them back. */
export function characterChanged(world: World, character: Character): World {
  return { ...world, character }
}

/**
 * A journey, as the command that made it reported it back (§3.12).
 *
 * The character and the trip arrive together and are stored together: a fare
 * paid and a place arrived at are one event, and a screen that had the new
 * balance beside the old journey would be describing two different moments.
 */
export function travelled(world: World, character: Character, trip: Trip): World {
  return { ...world, character, trip }
}

/**
 * The world layer, saying where in the painting the player ended up (§10.3).
 *
 * Written only by the renderer, and only when the answer changed — a walk
 * crossing a zone boundary, a location arriving, a district with no painting.
 * The comparison happens there, because the renderer is the only thing looking
 * every frame; this stores what it decided.
 */
export function stood(world: World, standing: Standing | null): World {
  return { ...world, standing }
}

/**
 * The effects tier, as somebody asked for it (§10.4).
 *
 * `chosen` distinguishes the floor from a decision: a device that has never been
 * told anything is at Low because that is where every device starts, and a
 * device at Low because its owner prefers it is a different sentence on screen.
 * Asking for a tier also clears the demotion — a machine is allowed to be told
 * twice, and a button that stopped working would be worse than a slow frame.
 */
export function qualityChosen(world: World, tier: EffectsTier, chosen: boolean): World {
  const { fps } = world.quality
  return { ...world, quality: { tier, chosen, demoted: false, fps } }
}

/**
 * The world could not hold the tier it was drawing at, and dropped back.
 *
 * The renderer's own frame clock said so (§10.4's `Governor`). Always to the
 * floor, and never in the other direction: climbing back up on its own would
 * make a machine near the line oscillate, and the player already has a button.
 */
export function qualityDemoted(world: World): World {
  return { ...world, quality: { ...world.quality, tier: FLOOR, chosen: false, demoted: true } }
}

/**
 * How fast the world is drawing.
 *
 * Keeps the same object when the number has not moved, which is what stops a
 * status readout from re-rendering a panel at the frame rate it is reporting.
 */
export function framesMeasured(world: World, fps: number): World {
  if (world.quality.fps === fps) return world
  return { ...world, quality: { ...world.quality, fps } }
}

/**
 * The panel as the server last stated it, and what today crossed.
 *
 * One function for every way the body can change — living a day, buying an
 * instrument — because in each case the server restates the whole panel and
 * this client stores it. There is nothing to merge and nothing to derive: an
 * instrument bought changes which metrics carry a reading, and a day lived
 * changes the readings, and both arrive as the same thing.
 */
export function panelChanged(world: World, health: Health, crossings: Crossing[] = []): World {
  return { ...world, health, crossings: crossings.length > 0 ? crossings : world.crossings }
}

/**
 * The rules, and where they lead, as the server last stated them.
 *
 * One patch for both, because the forecast is a reading of the body *under* the
 * rules — there is no moment at which one of them is current and the other is
 * not, and sending them apart would invite a screen showing a fortnight that
 * belongs to a configuration the player has already replaced.
 */
export function routinesChanged(world: World, routines: Routines, forecast: Forecast): World {
  return { ...world, routines, forecast }
}

/**
 * The world after the daily rollover advanced it (§3.3).
 *
 * The panel and the forecast together, for the same reason `routinesChanged`
 * sends them together: the forecast is a reading of the body, and a fortnight
 * projected from a body that has since lived four days is a fortnight about
 * nobody. The rules themselves are untouched — a rollover applies them, it does
 * not change them.
 *
 * `crossings` is whatever the *last* day applied moved across, not every day of
 * a catch-up. A threshold crossed three weeks ago is not news.
 */
export function rolledOver(
  world: World,
  health: Health,
  forecast: Forecast,
  crossings: Crossing[],
  character: Character,
): World {
  return { ...panelChanged(world, health, crossings), forecast, character }
}

/**
 * The job, as the server last stated it (§3.14).
 *
 * The whole surface on every change rather than the rung that moved, for the
 * reason §10.2 gives: what is held, what is above it and whether the step up is
 * open are one statement, and a patch carrying only the new tier would leave a
 * screen showing an interview door that had just closed behind the player.
 *
 * The character comes with it because taking or leaving a job changes what a day
 * is worth, and the two arriving apart would be a balance and a wage that
 * disagree for one render.
 */
export function employed(world: World, career: Career, character: Character): World {
  return { ...world, career, character }
}

/**
 * The playbook, as the server last stated it (§3.14).
 *
 * The whole of it arrives on every change rather than the one rule that moved,
 * for the reason §10.2 gives and one this feature adds: the vocabulary and the
 * rules are the same statement. A term unlocked by something the player did
 * changes which words the pools hold *and* whether an old rule still resolves,
 * and a patch that carried only the edited sentence would leave those two
 * disagreeing.
 */
export function ruled(world: World, playbook: Playbook): World {
  return { ...world, playbook }
}

/**
 * The composite, as the server last composed it (§3.5).
 *
 * It arrives on its own rather than folded into whatever moved it, because Karma
 * is a reading of a body, a balance and three hidden totals at once — no one
 * command owns it, and a `health.lived` payload carrying a Karma would be the
 * health module answering a question about money.
 */
export function composed(world: World, karma: Karma): World {
  return { ...world, karma }
}

/**
 * The boards, as `GET /api/boards` answered (§3.5).
 *
 * Fetched rather than hydrated, and stored rather than held by a component,
 * because the socket patches it: a rank moves when *somebody else* plays, and
 * the screen showing it has to hear about that without asking again.
 */
export function listed(world: World, boards: Boards): World {
  return { ...world, boards }
}

/**
 * A rebuild moved this player on one or more boards (§3.5, §8).
 *
 * Only the boards they actually moved on arrive, so this merges rather than
 * replaces — and it does nothing at all when the boards have never been fetched,
 * because a partial message cannot construct an index and the screen that opens
 * one fetches it anyway. That is the deliberate shape of a slice which is not
 * part of the boot snapshot: the socket keeps it current, it does not create it.
 */
export function ranked(
  world: World,
  refreshedAt: string,
  players: number,
  moved: { board: BoardId; to: number; of: number }[],
): World {
  if (world.boards === null) return world

  const by = new Map(moved.map((m) => [m.board.key, m]))
  return {
    ...world,
    boards: {
      refreshedAt,
      players,
      boards: world.boards.boards.map((placing) => {
        const move = by.get(placing.board.key)
        // The value is left alone. A rank moving says nothing about the reading
        // beside it, and the message deliberately does not carry one — least of
        // all for a sealed board, where there is no reading to carry.
        return move ? { ...placing, rank: move.to, of: move.of } : placing
      }),
    },
  }
}

/**
 * The endorsement surface, as the server last stated it (§3.5).
 *
 * One function for all three ways it can arrive — the game offered somebody, a
 * tab of this player's answered, or somebody else said they like them — because
 * in every case the server restates the whole surface and this stores it. There
 * is nothing to merge and nothing to increment: a client that added one to its
 * own `received` would be a client deriving a number the server already sent
 * (§10.2, rule 2).
 */
export function offered(world: World, endorsement: Endorsement): World {
  return { ...world, endorsement }
}

/**
 * The event surface, as the server last stated it (§4).
 *
 * One function for both ways it arrives — the world released something, or a tab
 * of this player's answered — because in both cases the server restates the
 * whole surface and this stores it. There is nothing to merge: an offer is
 * either standing or it is not, and only the server knows which.
 */
export function released(world: World, events: Events): World {
  return { ...world, events }
}

/**
 * The challenge surface, as the server last stated it (§5).
 *
 * Beginning, committing to a position and finishing are one patch, because the
 * server restates the whole surface for each of them — and because a client
 * holding a run and a doorway that came from different moments would be showing
 * a door into a challenge it is already inside.
 */
export function underway(world: World, challenge: Challenge): World {
  /*
   * Read the lists back the way the socket's reducer does.
   *
   * Go's nil slices arrive as null, and a *finished* run has no choices at all
   * — so a command response that ends a run hands this slice `choices: null`
   * where the socket path would have handed it `[]`. Two ways into one slice
   * with two different shapes is a bug generator, and it produced one: a screen
   * reading `choices[0]` crashed the whole tree the moment a run ended.
   */
  return {
    ...world,
    challenge: {
      ...challenge,
      run: challenge.run && {
        ...challenge.run,
        beats: challenge.run.beats ?? [],
        choices: challenge.run.choices ?? [],
        held: challenge.run.held ?? [],
      },
    },
  }
}

/**
 * What a delegated role has waiting for the player (§3.14).
 *
 * Restated whole rather than merged, because the server sends the whole surface
 * for the same reason every other slice is sent whole: at most one moment is
 * ever waiting, and "there is nothing now" is a thing that has to be sayable.
 */
export function delegated(world: World, situations: Situations): World {
  return { ...world, situations }
}

/**
 * What living among people has come to (§3.15).
 *
 * Restated whole rather than merged, for the same reason every other slice is:
 * at most one conflict is ever waiting, "there is nothing now" has to be
 * sayable, and the counts behind it move every time one is settled.
 */
export function living(world: World, conflicts: Conflicts): World {
  return { ...world, conflicts }
}

/**
 * The reducer table: one entry per message the socket can deliver.
 *
 * The mapped type is the point. `Push` is every server message except the
 * connection's own welcome — which the socket client handles itself and never
 * hands over — so adding a message type on the server regenerates that union,
 * and this object stops compiling until it has a reducer. That is the
 * "deliberate API decision" §10.2 asks every socket message to be.
 */
export type Push = Exclude<ServerMessage, { type: 'welcome' }>

type Reducers = {
  [M in Push as M['type']]: (world: World, payload: M['payload']) => World
}

const reducers: Reducers = {
  'character.moved': (world, payload) =>
    world.character === null
      ? world
      : {
          ...world,
          character: {
            ...world.character,
            at: receivedPlace(payload.at),
            money: payload.money,
          },
          trip: null,
        },

  // An instrument bought somewhere else, restating the panel because it changes
  // what the panel reads. Nothing here is a day being lived: the rollover owns
  // that and says so in its own message.
  'health.bought': (world, payload) => panelChanged(world, received(payload.panel)),

  // The rules changed somewhere else. Both halves are restated by the server,
  // so this stores rather than merges.
  'routine.set': (world, payload) =>
    routinesChanged(world, receivedRoutines(payload.routines), receivedForecast(payload.forecast)),

  // A rule composed somewhere else (§3.14). There is no second message for
  // saving it, because there is no save: the sentence the server restates here
  // is what the character does from the next time the situation comes up.
  'playbook.composed': (world, payload) => ruled(world, receivedPlaybook(payload.playbook)),

  // The world moved on its own: the daily rollover, at this player's local
  // midnight (§3.3). The only message no command of theirs caused, which is why
  // it is the clearest case for the socket existing at all — a tab left open
  // overnight would otherwise still be showing yesterday's body.
  //
  // The panel and the forecast are both restated, because a body that has moved
  // is a body whose readings and whose whole arc have moved with it.
  //
  // It also carries what a delegated role produced overnight (§3.14) and what
  // the people around them did (§3.15): the rollover is the only thing that can
  // produce either, so a tab left open would otherwise show a job with nothing
  // waiting on it, and a neighbourhood with nothing going on in it, until
  // somebody reloaded.
  'rollover.ran': (world, payload) =>
    living(
      delegated(
        rolledOver(
          world,
          received(payload.panel),
          receivedForecast(payload.forecast),
          payload.crossed ?? [],
          receivedCharacter(payload.character),
        ),
        receivedSituations(payload.situations),
      ),
      receivedConflicts(payload.conflicts),
    ),

  // The job changed — taken, left, or handed over at the end of an interview
  // (§3.14). One message for all three, because they are one event seen three
  // times: the rung moved.
  'career.changed': (world, payload) =>
    employed(world, receivedCareer(payload.career), receivedCharacter(payload.character)),

  // Progression moved — because a day was lived, a fare was paid, a wage was
  // earned, or the rollover advanced the world overnight. Sent to every tab
  // including the one that acted, since no command's response carries it.
  'factor.stood': (world, payload) => composed(world, receivedKarma(payload.karma)),

  // A rank moved, which is the one thing on this client that can change because
  // of somebody the player has never met (§8 named "leaderboard changes" among
  // the four things the socket exists to carry). Only the boards that moved are
  // sent, and only to the players who moved on them.
  'board.moved': (world, payload) =>
    ranked(
      world,
      payload.refreshedAt,
      payload.players,
      (payload.moved ?? []).map((m) => ({ board: receivedBoard(m.board), to: m.to, of: m.of })),
    ),

  // Peer endorsement (§3.5). Three messages rather than one, because they are
  // three different events even though they all restate the same slice: the
  // game named somebody, this player answered somewhere else, and — the one the
  // whole feature exists for — another human being said they like you.
  //
  // That last one is the first message in this game that is literally §8's
  // "another player's action affecting you". Nothing was asked for, nobody was
  // present, and there is no response it could have arrived in.
  'social.offered': (world, payload) => offered(world, receivedEndorsement(payload.endorsement)),
  'social.answered': (world, payload) => offered(world, receivedEndorsement(payload.endorsement)),
  'social.liked': (world, payload) => offered(world, receivedEndorsement(payload.endorsement)),

  // The world put something in front of the player (§4), and the world's own
  // decision is what did it — a walk arriving somewhere, or a page load. It is
  // sent with no origin for that reason: the tab whose walk triggered it has no
  // more claim on the news than any other, because it did not ask for this.
  'event.offered': (world, payload) => released(world, receivedEvents(payload.events)),
  // Answered somewhere else. The `after` and the name travel with it so a tab
  // that did not press anything can say what happened rather than only going
  // quiet.
  'event.answered': (world, payload) => released(world, receivedEvents(payload.events)),

  // A challenge run moved — begun, answered, or finished — somewhere else.
  // One message for all three, because they are one event seen three times:
  // the run is standing somewhere it was not. The tab that acted is skipped
  // server-side and applies its own command's response instead (§8, rule 3).
  'challenge.moved': (world, payload) => underway(world, receivedChallenge(payload.challenge)),

  // A moment a delegated role produced, answered somewhere else (§3.14). The
  // words travel with it so a tab that pressed nothing can say what the
  // character did and what came of it, rather than only going quiet — the same
  // reason `event.answered` carries its `after`.
  'situation.answered': (world, payload) =>
    delegated(world, receivedSituations(payload.situations)),

  // A conflict with somebody you live among, settled somewhere else (§3.15).
  // The words travel with it for the same reason the situation's do: a tab that
  // pressed nothing can still say what the character did and what came of it.
  'conflict.answered': (world, payload) =>
    living(world, receivedConflicts(payload.conflicts)),

  'probe.recorded': (world, payload) => ({
    ...world,
    probes: remember(world.probes, payload.probes, {
      label: payload.label,
      notedAt: payload.notedAt,
      // It arrived over the socket, so by definition this tab did not do it:
      // the originating session's own messages are skipped server-side (§8,
      // rule 4). Somewhere else means another tab, or another device.
      by: 'elsewhere',
    }),
  }),
}

/** Apply one socket message through its own reducer. */
export function applyMessage(world: World, message: Push): World {
  // The one cast in the file, and it is confined to this line. The table above
  // is what the type system checks — each reducer's payload is correlated with
  // its message type there — but an indexed lookup on a union loses that
  // correlation, and there is no way to express "this key's function takes this
  // key's payload" at the call site.
  const reduce = reducers[message.type] as (world: World, payload: unknown) => World
  return reduce(world, message.payload)
}

/** A place off the wire, with Go's nil slice of ways read back as a list. */
function receivedPlace(at: CharacterMoved['at']): Place {
  return {
    key: at.key,
    name: at.name,
    note: at.note,
    layer: at.layer as Place['layer'],
    in: at.in,
    stop: at.stop,
    ways: (at.ways ?? []).map((w) => ({
      to: w.to,
      mode: w.mode as Way['mode'],
      says: w.says,
      fare: w.fare,
    })),
  }
}

/**
 * A panel off the wire, as a panel.
 *
 * An empty Go slice is `null` in JSON rather than `[]`, which the generated
 * types state honestly and which nothing downstream should have to think about.
 * This is the one place that is turned back into a list.
 */
function received(panel: HealthBought['panel']): Health {
  return {
    biologicalAge: panel.biologicalAge,
    livedToday: panel.livedToday,
    tiers: panel.tiers ?? [],
    metrics: (panel.metrics ?? []).map((m) => ({ ...m, history: m.history ?? [] })),
  }
}

/** The routines off the wire, with Go's nil slices read back as empty lists. */
function receivedRoutines(routines: RoutineSet['routines']): Routines {
  return {
    set: routines.set,
    routines: (routines.routines ?? []).map((r) => ({ ...r, options: r.options ?? [] })),
  }
}

/**
 * The board off the wire, with Go's nil slice read back as a list.
 *
 * Note that the locked slots survive the trip intact: they arrive empty and are
 * stored empty. There is no branch here that decides what to keep, because there
 * is nothing to decide — the server sent five slots and three of them contain
 * nothing but the fact that they exist.
 */
function receivedKarma(karma: FactorStood['karma']): Karma {
  return {
    karma: karma.karma,
    slots: (karma.slots ?? []).map((s) => ({ ...s, reads: s.reads as Slot['reads'] })),
    curve: karma.curve ?? [],
    where: karma.where,
    among: karma.among,
    days: karma.days,
    familiarity: karma.familiarity,
  }
}

/**
 * The playbook off the wire, with Go's nil slices read back as lists.
 *
 * Note what is *not* done here. Nothing is merged with what the client already
 * had, and nothing is filled in: a `when` entry that is null stays null, because
 * a blank is a real state of a rule rather than a value to default. And no term
 * is added back — the pools arrive holding only what this player has unlocked,
 * and there is no branch below that could decide otherwise.
 */
function receivedPlaybook(playbook: PlaybookComposed['playbook']): Playbook {
  return {
    questions: (playbook.questions ?? []).map((q) => ({
      ...q,
      conditions: (q.conditions ?? []).map((t) => ({ ...t, values: t.values ?? [] })),
      actions: (q.actions ?? []).map((t) => ({ ...t, values: t.values ?? [] })),
    })),
    sets: (playbook.sets ?? []).map((s) => ({
      ...s,
      rules: (s.rules ?? []).map((r) => ({ ...r, when: r.when ?? [] })),
      conflicts: (s.conflicts ?? []).map((c) => ({
        ...c,
        questions: c.questions ?? [],
      })),
    })),
  }
}

/**
 * A board's identity off the wire.
 *
 * The only thing to do here is narrow `reads`, and note what is *not* done: a
 * sealed board arrives with an empty name and an empty `reads`, and is stored
 * that way. There is no branch that decides what to keep, because the server put
 * nothing in the fields a decision would be about.
 */
function receivedBoard(board: NonNullable<BoardMoved['moved']>[number]['board']): BoardId {
  return { ...board, reads: board.reads as BoardId['reads'] }
}

/**
 * The endorsement surface off the wire, with Go's nil slice read back as a list.
 *
 * Note what is *not* done here: nothing is compared against what the client
 * already had, and nothing is added to. The server states this whole surface on
 * every one of the three messages that carry it, so a reducer that merged would
 * be inventing a second opinion about a count somebody else caused.
 */
function receivedEndorsement(e: SocialOffered['endorsement']): Endorsement {
  return {
    offer: e.offer,
    given: e.given,
    cap: e.cap,
    opens: e.opens,
    received: e.received,
    liked: e.liked ?? [],
  }
}

/**
 * The event surface off the wire.
 *
 * A one-line reader, and it exists rather than the payload being stored
 * directly so that the store holds this file's type rather than a generated
 * one — the same reason every other `received*` below does.
 */
function receivedEvents(events: EventOffered['events']): Events {
  return { offer: events.offer }
}

/**
 * The delegated-work surface off the wire.
 *
 * Go's nil slice arrives as null, so the answers are read back as a list — the
 * same reading every other `received*` in this file does, and the reason the
 * store holds this file's types rather than generated ones.
 */
function receivedSituations(situations: SituationAnswered['situations']): Situations {
  return {
    waiting: situations.waiting && {
      ...situations.waiting,
      answers: situations.waiting.answers ?? [],
    },
  }
}

/**
 * The daily-conflict surface off the wire.
 *
 * Go's nil slices arrive as null, so every list here is read back as one — the
 * answers, the standing, what the population holds in common, the people, and
 * the pattern.
 */
function receivedConflicts(conflicts: ConflictAnswered['conflicts']): Conflicts {
  return {
    waiting: conflicts.waiting && {
      ...conflicts.waiting,
      standing: conflicts.waiting.standing ?? [],
      answers: conflicts.waiting.answers ?? [],
    },
    among: conflicts.among && {
      ...conflicts.among,
      holds: conflicts.among.holds ?? [],
      people: (conflicts.among.people ?? []).map((p) => ({ ...p, standing: p.standing ?? [] })),
      questions: conflicts.among.questions ?? [],
    },
  }
}

/**
 * The challenge surface off the wire.
 *
 * Go's nil slices arrive as null, so the beats and the choices are read back as
 * lists — the same reading every other `received*` in this file does, and the
 * reason the store holds this file's types rather than generated ones.
 */
function receivedChallenge(challenge: ChallengeMoved['challenge']): Challenge {
  return {
    here: challenge.here,
    run: challenge.run && {
      ...challenge.run,
      beats: challenge.run.beats ?? [],
      choices: challenge.run.choices ?? [],
      // The bag, for a mechanic that has one. Null off the wire when it is
      // empty, and read back as a list for the same reason the two above are.
      held: challenge.run.held ?? [],
    },
  }
}

/**
 * The character off the wire, whole.
 *
 * Used by the two messages that restate all of them rather than a field —
 * `career.changed`, because taking a job changes what a day is worth, and
 * `rollover.ran`, because the rollover is the one thing that moves a balance
 * with nobody asking (§3.4's structural income).
 */
function receivedCharacter(character: CareerChanged['character']): Character {
  return {
    money: character.money,
    at: receivedPlace(character.at),
    home: receivedPlace(character.home),
    timezone: character.timezone,
    earningsTaken: character.earningsTaken,
    earningsLeft: character.earningsLeft,
  }
}

/**
 * The job surface off the wire, with Go's nil slices read back as lists.
 *
 * The `state` on a requirement chip is a Go string and arrives as one, so it is
 * narrowed here — the one place in this file that knows the three words, and
 * the reason the store holds this file's types rather than generated ones.
 */
function receivedCareer(career: CareerChanged['career']): Career {
  const role = (r: NonNullable<CareerChanged['career']['held']>): Role => ({
    ...r,
    posting: {
      intro: r.posting.intro,
      duties: r.posting.duties ?? [],
      qualifications: r.posting.qualifications ?? [],
    },
  })
  const opening = (o: NonNullable<CareerChanged['career']['here']>): Opening => ({
    role: role(o.role),
    wants: (o.wants ?? []).map((w) => ({ label: w.label, state: w.state as Want['state'] })),
    ready: o.ready,
    walk: o.walk,
    take: o.take,
    interview: o.interview,
    experience: (o.experience ?? []).map((r) => ({
      needs: r.needs ?? [],
      short: r.short ?? [],
    })),
    worked: o.worked,
  })
  return {
    held: career.held && role(career.held),
    since: career.since,
    paid: career.paid,
    here: career.here && opening(career.here),
    board: (career.board ?? []).map(opening),
    work: (career.work ?? []).map(role),
  }
}

function receivedForecast(forecast: RoutineSet['forecast']): Forecast {
  return {
    days: forecast.days,
    biologicalAge: forecast.biologicalAge ?? [],
    metrics: (forecast.metrics ?? []).map((m) => ({ ...m, values: m.values ?? [] })),
  }
}

/**
 * Record a probe the server stated, whichever way it said it.
 *
 * The count comes from the message; the mark is this client's memory of having
 * been told. Note that the two are independent — the count is the whole table
 * and can move by more than one between marks, because other players probe too.
 */
function remember(
  probes: Probes,
  count: number,
  mark: Omit<Mark, 'n'>,
): Probes {
  const seen = probes.seen + 1
  const marks = [...probes.marks, { ...mark, n: seen }]
  return {
    count,
    seen,
    marks: marks.length > REMEMBERED ? marks.slice(marks.length - REMEMBERED) : marks,
  }
}
