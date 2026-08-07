import { describe, expect, it } from 'vitest'

import type { RolloverRan, SituationAnswered } from '../realtime/messages.gen'

import {
  applyMessage,
  framesMeasured,
  hydrated,
  initialWorld,
  listed,
  probed,
  qualityChosen,
  qualityDemoted,
  REMEMBERED,
  stood,
  travelled,
  underway,
  type Challenge,
  type World,
} from './state'

/*
 * The reducers, which are the part of the client worth testing on their own.
 *
 * They are pure by construction (§10.2) — no store, no fetch, no DOM — so what
 * is asserted below is the whole behaviour: what a statement from the server
 * does to the model, and what it deliberately leaves alone.
 *
 * The exhaustiveness of the reducer table is not tested here because it cannot
 * fail at runtime: a message type the table has no entry for is a compile
 * error, which is the point of writing it as a mapped type.
 */

const somewhere = {
  key: 'east-side',
  name: 'East Side',
  note: 'where you live',
  layer: 'district' as const,
  in: '',
  stop: true,
  ways: [
    { to: 'east-side-apartment', mode: 'foot' as const, says: 'in through the door', fare: 0 },
    { to: 'riverfront', mode: 'transit' as const, says: 'the Crosstown to Riverfront', fare: 275 },
  ],
}

const panel = {
  biologicalAge: 32,
  livedToday: false,
  tiers: [{ key: 'wearable', name: 'A good watch', buys: 'HRV', price: 40000, owned: false, forSale: true }],
  metrics: [
    {
      key: 'hrv',
      name: 'Heart-rate variability',
      unit: 'ms',
      means: 'the autonomic balance sheet',
      tier: 'wearable',
      tierName: 'A good watch',
      lower: false,
      decimals: 0,
      visible: false,
      value: null,
      history: [],
    },
  ],
}

/**
 * The rules for the body, as the server states them (§3.4).
 *
 * A routine is a standing question and a sentence answering it, which is the
 * same shape the playbook composes in (§3.14) — the payload is the interface
 * language as much as the screen is.
 */
const rules = {
  set: false,
  routines: [
    {
      key: 'sleep',
      name: 'Sleep',
      asks: 'What happens when the night gets late?',
      learn: 'A stretch of short nights.',
      unlocked: true,
      chosen: 'whenever',
      options: [
        { key: 'whenever', says: 'I sleep when I run out of day.', costs: 'Nothing, until it does.', chosen: true },
        { key: 'protected', says: 'Eight and a half hours.', costs: 'Evenings.', chosen: false },
      ],
    },
  ],
}

const ahead = {
  days: 2,
  biologicalAge: [31.4, 31.1],
  metrics: [
    { key: 'hrv', values: [] },
    { key: 'sleep', values: [8.5, 8.5] },
  ],
}

/**
 * The progression board (§3.5), exactly as the server sends it.
 *
 * Two slots carry a name and a reading; three carry nothing at all. That is the
 * fixture's whole point — it is not a redacted payload with the hidden numbers
 * stripped in the test, it is what arrives.
 */
const board = {
  karma: 214.8,
  slots: [
    {
      key: 'health',
      name: 'Biological age',
      reads: 'age' as const,
      note: 'composed from every biomarker, including the ones you cannot read',
      value: 32,
      shown: true,
    },
    {
      key: 'money',
      name: 'Money',
      reads: 'money' as const,
      note: 'what you are holding, not what you have earned',
      value: 120000,
      shown: true,
    },
    { key: '', name: '', reads: '' as const, note: 'something is being measured here', value: null, shown: false },
    { key: '', name: '', reads: '' as const, note: 'something is being measured here', value: null, shown: false },
    { key: '', name: '', reads: '' as const, note: 'something is being measured here', value: null, shown: false },
  ],
  where: 'east-side',
  days: 3,
  familiarity: 1,
}

/**
 * Peer endorsement (§3.5), as the snapshot carries it.
 *
 * Nobody to like right now, which is the state this surface is in most of the
 * time and is therefore the right default for a fixture: "occasional" has to
 * have a resting state, and it has to be the ordinary one.
 */
const nobodyToLike = {
  offer: null,
  given: 1,
  cap: 5,
  opens: '2026-08-06T20:00:00Z',
  received: 2,
  liked: [{ name: 'Ana', avatar: '' }],
}

// One environment, one question, and a rule with a hole in it — the state
// §3.14's loop begins in. The pools carry only what a new player has.
const composed = {
  questions: [
    {
      key: 'help',
      n: 1,
      ask: 'Somebody asks you for help with a task. When do you say yes?',
      note: 'It happens two or three times a week.',
      about: 'someone asks me for help',
      conditions: [
        { key: 'fee', says: 'they are paying me {value}', values: [{ key: 'fair', says: 'above $60' }] },
        { key: 'legal', says: 'the task is legal', values: [] },
      ],
      actions: [
        { key: 'yes', says: 'say yes', values: [] },
        { key: 'no', says: 'say no', values: [] },
      ],
    },
  ],
  sets: [
    {
      environment: 'east-side',
      name: 'East Side',
      rules: [
        {
          question: 'help',
          when: [null],
          join: 'and',
          then: { term: 'yes', value: '' },
          otherwise: null,
          inForce: false,
        },
      ],
      // Nothing to disagree with while nothing is in force, which is the
      // ordinary state of these two lists and the one the screen has to read
      // as ordinary.
      conflicts: [],
      idle: [],
    },
  ],
}

/**
 * Nothing in front of the player, which is the ordinary state of §4's surface
 * and the one a screen has to read as ordinary rather than as an empty state.
 */
const quiet = { offer: null }

/**
 * Nobody is part way through anything and nothing is standing in front of them,
 * which is what §5's surface says most of the time.
 */
const nothingUnderway = { here: null, run: null }

/**
 * Nothing a delegated role produced is waiting, which is what §3.14's surface
 * says while the player's own rules are keeping up with the job.
 */
const nothingWaiting = { waiting: null }

/**
 * One moment the rules could not settle: the roster gap, which is §3.14's own
 * example of the situation an over-general rule loses money on.
 */
const theRosterGap = {
  waiting: {
    id: 12,
    name: 'The roster gap',
    line: 'Priya is out until Thursday and the Friday night pick is nine bodies short.',
    role: 'Operations manager',
    environment: 'east-side',
    why: 'nothing in your playbook covered this.',
    on: '2026-08-05',
    at: '2026-08-06T09:00:00Z',
    answers: [
      { key: 'yield', does: 'Take the shift yourself and cover the gap.' },
      { key: 'bargain', does: 'Cover half of it, and put the other half in writing.' },
      { key: 'resist', does: 'Say no, and let Friday go out short.' },
    ],
  },
}

/**
 * Warehouse nights, and the desk above it out of reach — §3.14's ladder as a
 * player who has just taken the bottom rung sees it. The step up carries a
 * requirement with one blanked chip in it, which is the shape the screen has to
 * read: something else is being asked, and it does not say what.
 */
const nights = {
  tier: 1,
  key: 'warehouse-nights',
  name: 'Warehouse nights',
  does: 'Ten hours on the loading bay from ten at night.',
  costs: 'The night, every night.',
  pay: 8800,
  hours: 10,
  nights: true,
}

const desk = {
  tier: 2,
  key: 'dispatch',
  name: 'The dispatch desk',
  does: 'Nine hours in the daylight with a phone and a board.',
  costs: "Somebody else's emergency, all day.",
  pay: 15200,
  hours: 9,
  nights: false,
}

const onNights = {
  held: nights,
  since: '2026-08-01T09:00:00Z',
  paid: 8800,
  next: {
    role: desk,
    wants: [
      { label: 'Biological age', state: 'met' as const },
      { label: '', state: 'hidden' as const },
      { label: '', state: 'hidden' as const },
    ],
    ready: false,
    walk: null,
    interview: '',
  },
  here: null,
  ladder: [nights, desk],
}

const snapshot = {
  character: { money: 120000, at: somewhere, home: somewhere, earningsTaken: 0, earningsLeft: 3 },
  places: [somewhere],
  health: panel,
  routines: rules,
  forecast: ahead,
  playbook: composed,
  karma: board,
  endorsement: nobodyToLike,
  events: quiet,
  challenge: nothingUnderway,
  career: onNights,
  situations: nothingWaiting,
  probes: 7,
  database: '17.2',
  schema: 4,
}

function recorded(probes: number, label = '', at = '2026-08-06T12:00:00Z') {
  return {
    type: 'probe.recorded' as const,
    seq: 1,
    payload: { probes, label, notedAt: at },
  }
}

function result(probes: number, label = '', at = '2026-08-06T12:00:00Z') {
  return { probes, label, notedAt: at, database: '17.2', schema: 4 }
}

describe('hydrating over REST', () => {
  it('takes the count and the server it is talking to', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.probes.count).toBe(7)
    expect(world.server).toEqual({ database: '17.2', schema: 4 })
  })

  it('leaves marks alone — they are what this client was told, not what is', () => {
    const told = probed(initialWorld, result(1, 'first'))

    const world = hydrated(told, snapshot)

    expect(world.probes.count).toBe(7)
    expect(world.probes.marks).toBe(told.probes.marks)
  })

  // The renderer reads the store every frame and skips work by comparing
  // references. A reducer that cloned unconditionally would rebuild the scene
  // sixty times a second for nothing.
  it('keeps the slices it did not change', () => {
    const world = hydrated(initialWorld, snapshot)

    const again = hydrated(world, { ...snapshot })
    expect(again.probes).toBe(world.probes)
    expect(again.server).toBe(world.server)
  })
})

describe('a probe this tab sent', () => {
  it('takes the count from the response and remembers who did it', () => {
    const world = probed(initialWorld, result(3, 'mine'))

    expect(world.probes.count).toBe(3)
    expect(world.probes.marks).toHaveLength(1)
    expect(world.probes.marks[0]).toMatchObject({ n: 1, label: 'mine', by: 'you' })
  })
})

describe('a probe the socket brought', () => {
  it('is recorded as having happened somewhere else', () => {
    const world = applyMessage(initialWorld, recorded(2, 'theirs'))

    expect(world.probes.count).toBe(2)
    expect(world.probes.marks[0]).toMatchObject({ n: 1, label: 'theirs', by: 'elsewhere' })
  })

  // §8, rule 3 in the model: a command's response and a socket message are two
  // ways of being told, and both land in one place.
  it('numbers marks in the order this client learned of them', () => {
    let world: World = probed(initialWorld, result(1, 'mine'))
    world = applyMessage(world, recorded(2, 'theirs'))
    world = probed(world, result(3, 'mine again'))

    expect(world.probes.marks.map((m) => [m.n, m.by])).toEqual([
      [1, 'you'],
      [2, 'elsewhere'],
      [3, 'you'],
    ])
  })

  // The count is the whole table, so it can move by more than the marks do —
  // other players probe too, and this client is not told about theirs.
  it('takes whatever count the server stated, jump or not', () => {
    const world = applyMessage(probed(initialWorld, result(1)), recorded(40))

    expect(world.probes.count).toBe(40)
    expect(world.probes.marks).toHaveLength(2)
  })
})

describe('the bounded memory of marks', () => {
  it('keeps the most recent and drops the oldest', () => {
    let world = initialWorld
    for (let i = 1; i <= REMEMBERED + 5; i++) world = probed(world, result(i, `probe-${i}`))

    const marks = world.probes.marks
    expect(marks).toHaveLength(REMEMBERED)
    // Numbering counts every probe this client was told about, so the ones that
    // survived keep the numbers they were given — the renderer places marks by
    // that number, and a renumbering would move every mark on screen.
    expect(marks[0].n).toBe(6)
    expect(marks[marks.length - 1].n).toBe(REMEMBERED + 5)
    expect(world.probes.seen).toBe(REMEMBERED + 5)
  })
})

describe('the character', () => {
  const riverfront = {
    key: 'riverfront',
    name: 'Riverfront',
    note: 'where the money lives',
    layer: 'district',
    in: '',
    stop: true,
    ways: [{ to: 'downtown', mode: 'foot', says: 'on foot to Downtown', fare: 0 }],
  }
  const hydratedWorld = hydrated(initialWorld, snapshot)

  it('is hydrated with somewhere to stand, somewhere to live, and money', () => {
    expect(hydratedWorld.character).toEqual(snapshot.character)
    expect(hydratedWorld.places).toEqual(snapshot.places)
  })

  it('walks when another tab says it walked', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.moved',
      seq: 2,
      payload: { at: riverfront, by: 'foot', money: snapshot.character.money },
    })

    expect(world.character?.at).toEqual(riverfront)
    // Where they are, and what the fare left them with — both stated by the
    // server, because a ride moves money and a walk does not (§3.12).
    expect(world.character?.money).toBe(snapshot.character.money)
  })

  it('takes the fare from the message when another tab took the train', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.moved',
      seq: 3,
      payload: { at: riverfront, by: 'transit', money: 119725 },
    })

    expect(world.character?.money).toBe(119725)
    // The journey belongs to the tab that made it. This one only learns where
    // the player ended up.
    expect(world.trip).toBeNull()
  })

  // Go's nil slice is null on the wire. It becomes a list here and nowhere else.
  it('reads a place with no ways out as a place with no ways out', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.moved',
      seq: 4,
      payload: { at: { ...riverfront, ways: null }, by: 'foot', money: 1 },
    })

    expect(world.character?.at.ways).toEqual([])
  })

  // Every number the message carries was stated by the server, including how
  // many paid actions are left — the client counts nothing (§10.2, rule 2).
  it('takes both halves of the cap from the message rather than counting', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.earned',
      seq: 3,
      payload: { money: 124500, earned: 4500, earningsTaken: 1, earningsLeft: 2 },
    })

    expect(world.character?.money).toBe(124500)
    expect(world.character?.earningsTaken).toBe(1)
    expect(world.character?.earningsLeft).toBe(2)
  })

  // A journey is stored with the character it moved (§3.12): a fare paid and a
  // place arrived at are one event.
  it('stores a journey beside the character it moved', () => {
    const world = travelled(hydratedWorld, { ...snapshot.character, money: 119725 }, {
      by: 'transit',
      steps: [{ to: 'riverfront', mode: 'transit', says: 'the Crosstown to Riverfront' }],
      fare: 275,
      chances: 0,
    })

    expect(world.character?.money).toBe(119725)
    expect(world.trip?.fare).toBe(275)
    // The whole of what the fare bought: nothing happened on the way.
    expect(world.trip?.chances).toBe(0)
  })

  it('records a walk as free, and as somewhere things could have happened', () => {
    const world = travelled(hydratedWorld, snapshot.character, {
      by: 'foot',
      steps: [
        { to: 'downtown', mode: 'foot', says: 'on foot to Downtown' },
        { to: 'riverfront', mode: 'foot', says: 'on foot to Riverfront' },
      ],
      fare: 0,
      chances: 2,
    })

    expect(world.trip?.fare).toBe(0)
    expect(world.trip?.chances).toBe(2)
  })

  // A message about a character this client has not been told about yet can
  // only be applied to nothing. It is dropped rather than inventing one.
  it('ignores a change that arrives before there is anybody to change', () => {
    const world = applyMessage(initialWorld, {
      type: 'character.moved',
      seq: 1,
      payload: { at: riverfront, by: 'foot', money: 0 },
    })

    expect(world).toBe(initialWorld)
  })
})

/*
 * Where in the painting the player is standing (§10.3).
 *
 * The one slice no server statement touches. It is written by the renderer,
 * from authored geometry it loaded itself, and it carries no game rule — which
 * rectangle a walk ended inside is a fact about a Tiled file. What *happens*
 * in a zone is the event system's (#21, #22), and these keys are the vocabulary
 * it will be asked in.
 */
describe('standing somewhere', () => {
  const stoop = {
    level: 'east-side-street',
    zones: [
      { key: 'home-door', name: 'Your stoop' },
      { key: 'on-foot', name: 'On foot, the East Side' },
    ],
    portals: [{ key: 'east-side-apartment', name: 'Your apartment', to: 'east-side-apartment' }],
  }

  it('is nowhere until the world has drawn a location', () => {
    expect(initialWorld.standing).toBeNull()
  })

  it('records the zones and the ways out, most specific zone first', () => {
    const world = stood(hydrated(initialWorld, snapshot), stoop)

    expect(world.standing?.zones[0].key).toBe('home-door')
    expect(world.standing?.portals[0].to).toBe('east-side-apartment')
  })

  it('says nothing about the character, who has not moved district', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = stood(before, stoop)

    expect(world.character).toBe(before.character)
    expect(world.health).toBe(before.health)
  })

  // Walking into a district nobody has painted, or signing out: both leave the
  // world with no location on it, and nowhere is the honest answer.
  it('goes back to nowhere when there is no location on screen', () => {
    const world = stood(hydrated(initialWorld, snapshot), stoop)

    expect(stood(world, null).standing).toBeNull()
  })
})

/*
 * The rules for the body (§3.4), which arrive by both routes: hydrated with the
 * world at boot, and pushed when another tab states them.
 */
/*
 * The effects tier (§10.4). The one slice of the model that is a fact about the
 * device rather than about the world, and the reducers are where the two rules
 * that keep it honest live: asking clears a demotion, and a demotion only ever
 * goes downwards.
 */
describe('how the world is drawn', () => {
  it('starts on the floor, unchosen, and unmeasured', () => {
    expect(initialWorld.quality).toEqual({
      tier: 'low',
      chosen: false,
      demoted: false,
      fps: 0,
    })
  })

  it('takes the tier that was asked for, and keeps what was measured', () => {
    const measured = framesMeasured(initialWorld, 58)
    const world = qualityChosen(measured, 'high', true)

    expect(world.quality.tier).toBe('high')
    expect(world.quality.chosen).toBe(true)
    expect(world.quality.fps).toBe(58)
  })

  it('drops back to the floor when the machine could not hold it', () => {
    const world = qualityDemoted(qualityChosen(initialWorld, 'high', true))

    expect(world.quality.tier).toBe('low')
    expect(world.quality.demoted).toBe(true)
    // Not chosen: the machine decided this, not the player.
    expect(world.quality.chosen).toBe(false)
  })

  it('lets a machine be told twice', () => {
    const dropped = qualityDemoted(qualityChosen(initialWorld, 'high', true))
    const again = qualityChosen(dropped, 'high', true)

    // A button that stopped working after one refusal would be worse than a
    // slow frame.
    expect(again.quality).toEqual({ tier: 'high', chosen: true, demoted: false, fps: 0 })
  })

  it('leaves the world alone when the frame rate has not moved', () => {
    const world = framesMeasured(initialWorld, 60)
    // The renderer reports this from its own loop, and a new object every frame
    // would be a React render every frame.
    expect(framesMeasured(world, 60)).toBe(world)
    expect(framesMeasured(world, 59)).not.toBe(world)
  })

  it('says nothing about the world it is drawing', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = qualityChosen(before, 'high', true)

    expect(world.character).toBe(before.character)
    expect(world.places).toBe(before.places)
    expect(world.health).toBe(before.health)
  })
})

describe('the routines', () => {
  it('are hydrated with the fortnight they lead to', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.routines?.set).toBe(false)
    expect(world.routines?.routines[0].asks).toBe('What happens when the night gets late?')
    expect(world.forecast?.days).toBe(2)
    expect(world.forecast?.biologicalAge).toEqual([31.4, 31.1])
  })

  // A projection is a reading of a body, so it obeys the panel's rule: what is
  // withheld is the reading, not the idea that the measurement exists (§3.16).
  it('carry no projected values for a metric with no instrument behind it', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.forecast?.metrics.find((m) => m.key === 'hrv')?.values).toEqual([])
    expect(world.forecast?.metrics.find((m) => m.key === 'sleep')?.values).toEqual([8.5, 8.5])
  })

  it('are restated whole when another tab sets them', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, {
      type: 'routine.set',
      seq: 4,
      payload: {
        routines: {
          set: true,
          routines: [{ ...rules.routines[0], chosen: 'protected', options: null }],
        },
        forecast: { days: 2, biologicalAge: [30.9, 30.2], metrics: null },
      },
    })

    expect(world.routines?.set).toBe(true)
    expect(world.routines?.routines[0].chosen).toBe('protected')
    // An empty Go slice is null on the wire, and this is the one place that is
    // turned back into a list — nothing downstream should have to think about it.
    expect(world.routines?.routines[0].options).toEqual([])
    expect(world.forecast?.metrics).toEqual([])
    expect(world.forecast?.biologicalAge).toEqual([30.9, 30.2])

    // The rules changing says nothing about the body. The panel moves when a
    // day is lived, which is a different message.
    expect(world.health).toBe(before.health)
  })
})

/*
 * The playbook (§3.14): rules for judgement, set per environment (§3.15).
 *
 * Two things are worth asserting in the model rather than on the server. A blank
 * is a real state of a rule and survives the trip as one — nothing here fills it
 * in — and the vocabulary arrives already holding only what this player has,
 * so there is no branch in this client that could decide otherwise.
 */
describe('the playbook', () => {
  it('is hydrated as a rule set per environment, none of it in force', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.playbook?.sets).toHaveLength(1)
    expect(world.playbook?.sets[0].environment).toBe('east-side')
    expect(world.playbook?.sets[0].rules[0].inForce).toBe(false)
    // The hole the player has yet to fill, kept as a hole.
    expect(world.playbook?.sets[0].rules[0].when).toEqual([null])
    expect(world.playbook?.sets[0].rules[0].otherwise).toBeNull()
  })

  it('carries only the words this player has, and no count of the ones they have not', () => {
    const world = hydrated(initialWorld, snapshot)
    const help = world.playbook?.questions[0]

    expect(help?.conditions.map((t) => t.key)).toEqual(['fee', 'legal'])
    expect(help?.conditions[1].values).toEqual([])
    expect(JSON.stringify(world.playbook)).not.toContain('locked')
  })

  it('is restated whole when a rule is composed in another tab', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, {
      type: 'playbook.composed',
      seq: 9,
      payload: {
        playbook: {
          questions: [{ ...composed.questions[0], conditions: null, actions: null }],
          sets: [
            {
              environment: 'east-side',
              name: 'East Side',
              rules: [
                {
                  question: 'help',
                  when: [{ term: 'fee', value: 'fair' }],
                  join: 'and',
                  then: { term: 'yes', value: '' },
                  otherwise: { term: 'no', value: '' },
                  inForce: true,
                },
              ],
              conflicts: null,
              idle: null,
            },
          ],
        },
      },
    })

    expect(world.playbook?.sets[0].rules[0].inForce).toBe(true)
    expect(world.playbook?.sets[0].rules[0].when[0]?.value).toBe('fair')
    // An empty Go slice is null on the wire, and this is the one place that is
    // turned back into a list.
    expect(world.playbook?.questions[0].conditions).toEqual([])
    expect(world.playbook?.sets[0].conflicts).toEqual([])

    // Composing a rule says nothing about the body or the balance: it is a
    // statement about what the character will do, not about what has happened.
    expect(world.health).toBe(before.health)
    expect(world.character).toBe(before.character)
  })

  /*
   * Two answers that could disagree arrive with the rules, worked out by the
   * server before any of it happened (technical.md §6).
   *
   * The sentence comes down written, and that is the assertion worth making
   * here: there is no branch in this reducer that could compose one, so there is
   * no way for the client to end up telling the player something the matcher
   * would not do (§10.2, rule 2).
   */
  it('keeps what the server said two answers would do when they meet', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, {
      type: 'playbook.composed',
      seq: 10,
      payload: {
        playbook: {
          questions: null,
          sets: [
            {
              environment: 'east-side',
              name: 'East Side',
              rules: null,
              conflicts: [
                {
                  questions: ['help', 'money'],
                  n: [1, 3],
                  always: true,
                  says: 'Your answers to 1 and 3 always disagree.',
                },
              ],
              idle: [{ question: 'money', n: 3, says: 'Answer 3 does the same thing either way.' }],
            },
          ],
        },
      },
    })

    const set = world.playbook?.sets[0]
    expect(set?.conflicts[0].n).toEqual([1, 3])
    expect(set?.conflicts[0].always).toBe(true)
    expect(set?.conflicts[0].says).toBe('Your answers to 1 and 3 always disagree.')
    expect(set?.idle[0].question).toBe('money')
    // No posture, and no threshold. The two things a client could predict the
    // next decision from have no field to arrive in.
    expect(JSON.stringify(set)).not.toContain('yield')
    expect(JSON.stringify(set)).not.toContain('resist')
  })
})

/*
 * The daily rollover (§3.3): the world moving with nobody asking.
 *
 * It is the only message no command of this client's caused, which makes it the
 * clearest case for the socket existing at all — a tab left open overnight
 * would otherwise still be showing yesterday's body.
 */
describe('the world rolling over', () => {
  const rolled = (crossed: { metric: string; from: string; to: string; worse: boolean }[] | null) => ({
    type: 'rollover.ran' as const,
    seq: 9,
    payload: {
      panel: { ...panel, biologicalAge: 33.6, livedToday: false },
      forecast: { days: 2, biologicalAge: [33.4, 33.2], metrics: null },
      crossed,
      days: 4,
      skipped: 0,
      // The rollover pays every wage this game has (§3.4's structural income),
      // so it restates the character too — four nights on the loading bay, at
      // the bottom rung's day rate.
      character: {
        money: 120000 + 4 * 8800,
        at: somewhere,
        home: somewhere,
        earningsTaken: 0,
        earningsLeft: 3,
      },
      // And what the job did while nobody was in it (§3.14). The rollover is
      // the only thing that can produce one, which is why it carries them.
      situations: theRosterGap,
    },
  })

  it('brings the wages the days paid, which nothing else in the game does', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, rolled(null))

    expect(world.character?.money).toBe(120000 + 4 * 8800)
    // And the cap is untouched: a wage is not a paid action (§3.4).
    expect(world.character?.earningsLeft).toBe(3)
  })

  it('restates the panel and the fortnight it now leads to', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, rolled(null))

    expect(world.health?.biologicalAge).toBe(33.6)
    expect(world.forecast?.biologicalAge).toEqual([33.4, 33.2])
    // A rollover applies the rules; it does not change them.
    expect(world.routines).toBe(before.routines)
  })

  it('says what the last day applied crossed', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(
      before,
      rolled([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }]),
    )

    expect(world.crossings).toEqual([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }])
  })

  it('leaves the last crossings alone when a quiet day crossed nothing', () => {
    const before = applyMessage(
      hydrated(initialWorld, snapshot),
      rolled([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }]),
    )
    const world = applyMessage(before, rolled(null))

    expect(world.crossings).toBe(before.crossings)
  })
})

/*
 * Progression (§3.5): the composite, and the factors nobody is shown.
 *
 * There is no level in this game, and there is no level bar in this model. What
 * the reducer stores is a number the server composed on read, and a board whose
 * hidden slots are empty on arrival rather than emptied here.
 */
describe('the progression board', () => {
  const moved = (karma: number) => ({
    type: 'factor.stood' as const,
    seq: 11,
    payload: { karma: { ...board, karma, slots: board.slots } },
  })

  it('arrives with the world and is replaced whenever the server recomposes it', () => {
    const before = hydrated(initialWorld, snapshot)
    expect(before.karma?.karma).toBe(214.8)

    const world = applyMessage(before, moved(198.2))
    expect(world.karma?.karma).toBe(198.2)
  })

  // §3.5: the composite can fall, which is what keeps it from being a level bar
  // wearing a different name. Nothing in this client clamps or ratchets it.
  it('lets the composite fall', () => {
    const before = applyMessage(hydrated(initialWorld, snapshot), moved(300))
    const world = applyMessage(before, moved(12))

    expect(world.karma?.karma).toBe(12)
  })

  // The strongest assertion in the file: there is nothing on the wire to reveal.
  it('carries three slots with no name, no key and no reading', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), moved(214.8))
    const hidden = world.karma?.slots.filter((s) => !s.shown) ?? []

    expect(hidden).toHaveLength(3)
    for (const slot of hidden) {
      expect(slot.key).toBe('')
      expect(slot.name).toBe('')
      expect(slot.value).toBeNull()
    }
  })

  it('keeps the two that are shown, as the readings the design surfaces them as', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), moved(214.8))
    const shown = world.karma?.slots.filter((s) => s.shown) ?? []

    expect(shown.map((s) => [s.key, s.reads])).toEqual([
      ['health', 'age'],
      ['money', 'money'],
    ])
  })

  // §3.15: the environment wears out, and the player has to be able to feel it.
  it('says how worn the place they are standing in is', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), {
      type: 'factor.stood' as const,
      seq: 12,
      payload: { karma: { ...board, days: 40, familiarity: 0.21 } },
    })

    expect(world.karma?.where).toBe('east-side')
    expect(world.karma?.familiarity).toBeCloseTo(0.21)
  })

  // Go sends an empty slice as null. Nothing downstream should have to know.
  it('reads a null board back as a list', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), {
      type: 'factor.stood' as const,
      seq: 13,
      payload: { karma: { ...board, slots: null } },
    })

    expect(world.karma?.slots).toEqual([])
  })
})

/*
 * The leaderboards (§3.5).
 *
 * The distributed board is the one slice of this model the boot snapshot does
 * not carry — it is a read-only paginated list of other people (§10.2), fetched
 * when somebody looks at it. What the socket does is move the player's own place
 * on it, because a rank changes when *somebody else* plays.
 */
describe('the leaderboards', () => {
  const sealed = (n: number) => ({
    key: `slot-${n}`,
    name: '',
    note: 'a real ranking of something nobody has told you the name of',
    reads: '' as const,
    shown: false,
  })

  const karmaBoard = {
    key: 'karma',
    name: 'Karma',
    note: 'everything at once',
    reads: 'karma' as const,
    shown: true,
  }

  const index = {
    refreshedAt: '2026-08-07T09:00:00Z',
    players: 3,
    boards: [
      { board: karmaBoard, rank: 2, of: 3, value: 214.8 },
      { board: sealed(3), rank: 3, of: 3, value: null },
    ],
  }

  const rebuild = (moved: unknown[], refreshedAt = '2026-08-07T09:05:00Z') => ({
    type: 'board.moved' as const,
    seq: 21,
    payload: { refreshedAt, players: 4, moved } as never,
  })

  it('is not part of the boot snapshot', () => {
    expect(hydrated(initialWorld, snapshot).boards).toBeNull()
  })

  it('stores the index the boards screen fetched', () => {
    const world = listed(initialWorld, index)
    expect(world.boards?.players).toBe(3)
    expect(world.boards?.boards.map((p) => p.board.key)).toEqual(['karma', 'slot-3'])
  })

  it('moves the ranks a rebuild says moved, and leaves the others alone', () => {
    const world = applyMessage(
      listed(initialWorld, index),
      rebuild([{ board: karmaBoard, from: 2, to: 1, of: 4 }]),
    )

    expect(world.boards?.boards[0].rank).toBe(1)
    expect(world.boards?.boards[0].of).toBe(4)
    // Untouched, because the message said nothing about it.
    expect(world.boards?.boards[1].rank).toBe(3)
    expect(world.boards?.boards[1].of).toBe(3)
    // And the whole index now dates from the rebuild that moved them.
    expect(world.boards?.refreshedAt).toBe('2026-08-07T09:05:00Z')
    expect(world.boards?.players).toBe(4)
  })

  /*
   * §3.5's claim, stated as a test: a ranking discloses nothing.
   *
   * A sealed board can move a player up the table and still carry no name, no
   * `reads` and no value — there is nothing in the message for this client to
   * decide to withhold.
   */
  it('moves a player on a board that has no name and no number', () => {
    const world = applyMessage(
      listed(initialWorld, index),
      rebuild([{ board: sealed(3), from: 3, to: 1, of: 4 }]),
    )

    const moved = world.boards?.boards[1]
    expect(moved?.rank).toBe(1)
    expect(moved?.board.name).toBe('')
    expect(moved?.board.reads).toBe('')
    expect(moved?.value).toBeNull()
  })

  // The socket keeps this slice current; it does not create it. A rank that
  // moved before anybody opened the screen is not worth reconstructing from a
  // partial message, because opening it fetches the whole thing.
  it('ignores a rebuild when the boards have never been fetched', () => {
    const world = applyMessage(initialWorld, rebuild([{ board: karmaBoard, from: 0, to: 1, of: 4 }]))
    expect(world.boards).toBeNull()
    expect(world).toBe(initialWorld)
  })

  // Go sends an empty slice as null.
  it('reads a null list of movements back as none', () => {
    const world = applyMessage(listed(initialWorld, index), rebuild(null as never))
    expect(world.boards?.boards[0].rank).toBe(2)
    expect(world.boards?.refreshedAt).toBe('2026-08-07T09:05:00Z')
  })
})

/*
 * Peer endorsement (§3.5).
 *
 * The one slice of this model that another human being moves. Three messages
 * carry it — the game named somebody, this player answered somewhere else, and
 * somebody said they like them — and all three restate the whole surface, which
 * is the property these tests are really about: there is no counter incremented
 * here, so a client cannot end up with a number the server never said.
 */
describe('peer endorsement', () => {
  const anOffer = {
    offer: { id: 41, who: { name: 'Ilya', avatar: '' }, until: '2026-08-06T20:00:00Z' },
    given: 1,
    cap: 5,
    opens: '',
    received: 2,
    liked: [{ name: 'Ana', avatar: '' }],
  }

  const message = (type: 'social.offered' | 'social.answered', endorsement: unknown) =>
    ({ type, seq: 20, payload: { endorsement } }) as never

  it('arrives with the world, resting, and says when the next one comes', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.endorsement?.offer).toBeNull()
    expect(world.endorsement?.opens).toBe('2026-08-06T20:00:00Z')
    expect(world.endorsement?.given).toBe(1)
    expect(world.endorsement?.cap).toBe(5)
  })

  it('takes an offer the game made and drops it again when it is answered', () => {
    const offered = applyMessage(hydrated(initialWorld, snapshot), message('social.offered', anOffer))
    expect(offered.endorsement?.offer?.who.name).toBe('Ilya')
    // Nothing but the offer's own id addresses the other player.
    expect(offered.endorsement?.offer?.id).toBe(41)

    const answered = applyMessage(
      offered,
      message('social.answered', { ...anOffer, offer: null, given: 2, opens: '2026-08-07T09:00:00Z' }),
    )
    expect(answered.endorsement?.offer).toBeNull()
    expect(answered.endorsement?.given).toBe(2)
  })

  /*
   * The message the whole feature exists for.
   *
   * Nobody asked for it, nobody was present, and no command's response could
   * have carried it — §8's "another player's action affecting you", arriving for
   * the first time in this game. The count comes off the wire whole: this client
   * does not add one to what it had.
   */
  it('stores what the server says when somebody else says they like you', () => {
    const before = applyMessage(hydrated(initialWorld, snapshot), message('social.offered', anOffer))

    const world = applyMessage(before, {
      type: 'social.liked' as const,
      seq: 21,
      payload: {
        by: { name: 'Ilya', avatar: '' },
        endorsement: {
          ...anOffer,
          received: 3,
          liked: [
            { name: 'Ilya', avatar: '' },
            { name: 'Ana', avatar: '' },
          ],
        },
      },
    })

    expect(world.endorsement?.received).toBe(3)
    expect(world.endorsement?.liked.map((p) => p.name)).toEqual(['Ilya', 'Ana'])
    // The offer this player is holding is untouched by somebody else's action.
    expect(world.endorsement?.offer?.id).toBe(41)
  })

  // Go sends an empty slice as null. Nothing downstream should have to know.
  it('reads a null list of admirers back as a list', () => {
    const world = applyMessage(
      hydrated(initialWorld, snapshot),
      message('social.offered', { ...anOffer, liked: null, received: 0 }),
    )
    expect(world.endorsement?.liked).toEqual([])
  })
})

/*
 * The event surface (§4).
 *
 * Two messages, one slice, and no arithmetic anywhere: an offer is standing or
 * it is not, and only the server knows which. What these tests are really about
 * is the *absence* of the backlog — there is nothing in this model that says how
 * much the world has ready, because a client that knew could walk towards it.
 */
describe('what the world puts in front of you', () => {
  const anEvent = {
    id: 77,
    name: 'A knock at the door',
    kind: 'interpersonal',
    line: 'Somebody knocks.',
    accept: 'Open the door',
    decline: 'Stand very still',
    place: 'east-side-apartment',
    zone: 'at-home',
    until: '2026-08-06T20:45:00Z',
  }

  const message = (type: 'event.offered' | 'event.answered', offer: unknown) =>
    ({ type, seq: 30, payload: { events: { offer }, answer: 'declined', name: '', after: '' } }) as never

  it('arrives with the world, quiet, which is the ordinary state', () => {
    const world = hydrated(initialWorld, snapshot)
    expect(world.events).not.toBeNull()
    expect(world.events?.offer).toBeNull()
  })

  it('takes what the world released and puts it where it was placed', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), message('event.offered', anEvent))

    expect(world.events?.offer?.name).toBe('A knock at the door')
    // The spot is a named trigger zone (§10.3), which is what lets a painting
    // draw a marker in the right doorway.
    expect(world.events?.offer?.place).toBe('east-side-apartment')
    expect(world.events?.offer?.zone).toBe('at-home')
    // The words are the server's, not a key to look up in a copy of the
    // catalogue this client does not have.
    expect(world.events?.offer?.accept).toBe('Open the door')
  })

  /*
   * Answering it takes it off screen — and that is *all* it does here.
   *
   * A declined event is back in the world's backlog, which this client is never
   * told and must never assume it can work out. The absence of an offer is the
   * whole of what a client knows after an answer.
   */
  it('leaves nothing behind when it is answered somewhere else', () => {
    const standing = applyMessage(hydrated(initialWorld, snapshot), message('event.offered', anEvent))
    const answered = applyMessage(standing, message('event.answered', null))

    expect(answered.events?.offer).toBeNull()
    // Nothing else moved. An answer is not news about the body or the money.
    expect(answered.health).toBe(standing.health)
    expect(answered.character).toBe(standing.character)
  })

  // The model holds one moment and no queue. Asserted as a shape, because the
  // temptation to add "3 waiting" to a screen is exactly what §4 rules out.
  it('carries no backlog at all', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), message('event.offered', anEvent))
    expect(Object.keys(world.events ?? {})).toEqual(['offer'])
  })
})

/*
 * A challenge, on the client (§5).
 *
 * Three claims, and the last of them is the one worth having a test for.
 *
 * The surface holds a run or a door and never both; the run is whatever the
 * server last said and nothing here computes any part of it; and a *finished*
 * run survives a hydration that no longer carries it — because the reaction the
 * player is reading at that moment is the only account of a six-minute
 * endeavour they will ever get, and re-reading the world a second later must
 * not take it back.
 */
describe('a challenge', () => {
  const scene = {
    challenge: 'resolve_conflict/a',
    name: 'The argument on the stoop',
    scene: 'laying-it-out',
    where: 'Still the steps',
    taken: 'Marisol moves her bag off the step to make room for you.',
    beats: [{ who: 'Dev', line: 'I took a form.' }],
    ask: 'Choose the reading you will commit to.',
    choices: [
      { key: 'money', text: 'Say it looks to you like a number.' },
      { key: 'leaving', text: 'Say she is not arguing about money at all.' },
    ],
    stage: 2,
    stages: 5,
    done: false,
    closing: '',
    began: '2026-08-06T18:45:00Z',
    moved: '2026-08-06T18:47:00Z',
  }

  const moved = (challenge: unknown) =>
    ({ type: 'challenge.moved', seq: 40, payload: { challenge } }) as never

  it('arrives with the world, with nothing underway', () => {
    const world = hydrated(initialWorld, snapshot)
    expect(world.challenge).toEqual({ here: null, run: null })
  })

  it('takes the scene the server stated, and computes no part of it', () => {
    const world = applyMessage(
      hydrated(initialWorld, snapshot),
      moved({ here: null, run: scene }),
    )

    expect(world.challenge?.run?.scene).toBe('laying-it-out')
    expect(world.challenge?.run?.choices).toHaveLength(2)
    expect(world.challenge?.run?.ask).toBe('Choose the reading you will commit to.')
    // How far through is a length the server sent, never a count kept here.
    expect(world.challenge?.run?.stage).toBe(2)
    // And nothing on a choice says what it is worth, because nothing on the
    // wire does (§3.10's hidden scoring).
    expect(Object.keys(world.challenge?.run?.choices[0] ?? {})).toEqual(['key', 'text'])
  })

  // Go sends nil slices as null. A scene with nobody speaking is still a scene.
  it('reads a scene with no lists back as a scene with empty ones', () => {
    const world = applyMessage(
      hydrated(initialWorld, snapshot),
      moved({ here: null, run: { ...scene, beats: null, choices: null } }),
    )
    expect(world.challenge?.run?.beats).toEqual([])
    expect(world.challenge?.run?.choices).toEqual([])
  })

  it('keeps the ending on screen when the world is read again', () => {
    const ended: Challenge = {
      here: null,
      run: { ...scene, done: true, choices: [], ask: '', closing: 'They did not sign.' },
    }
    const finished = underway(hydrated(initialWorld, snapshot), ended)

    // The snapshot no longer carries it: a finished run is not what the world
    // is currently doing, and the server says so by sending neither half.
    const again = hydrated(finished, snapshot)
    expect(again.challenge?.run?.closing).toBe('They did not sign.')

    // But anything the server *does* say about that surface replaces it.
    const next = hydrated(finished, {
      ...snapshot,
      challenge: { here: { challenge: 'x/a', name: 'Something else', place: 'p', zone: 'z' }, run: null },
    })
    expect(next.challenge?.run).toBeNull()
    expect(next.challenge?.here?.name).toBe('Something else')
  })
})

/*
 * The job (§3.14): the rung held, the rung above it, and the whole ladder.
 *
 * The thing worth holding here is the veil. A rung's requirement is a floor on
 * all five factors and three of them are ones the player is never told exist
 * (§3.5), so what arrives is chips — two of them named, the rest blank — plus
 * one boolean that was decided on the server against floors this client never
 * sees. Nothing in the model can work out what is missing, and that is the
 * point rather than a limitation.
 */
describe('the job', () => {
  const changed = (career: unknown, character: unknown = snapshot.character) =>
    ({ type: 'career.changed', seq: 50, payload: { career, character } }) as never

  it('arrives with the world, held rung and all', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.career?.held?.name).toBe('Warehouse nights')
    expect(world.career?.paid).toBe(8800)
    // Every rung, including the ones this player cannot have: §3.14's ladder
    // teaches the trade by being visible.
    expect(world.career?.ladder).toHaveLength(2)
  })

  it('says something is being asked for without saying what', () => {
    const world = hydrated(initialWorld, snapshot)
    const wants = world.career?.next?.wants ?? []

    expect(wants).toHaveLength(3)
    expect(wants.filter((w) => w.state === 'hidden')).toHaveLength(2)
    // A blanked chip carries no name and no verdict. If it carried either, the
    // moment it changed would name the factor behind it.
    expect(wants.filter((w) => w.state === 'hidden').every((w) => w.label === '')).toBe(true)
    // And whether the step up is open is one boolean the server decided.
    expect(world.career?.next?.ready).toBe(false)
  })

  it('takes the rung and the balance together when the job moves', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(
      before,
      changed(
        { ...onNights, held: desk, paid: 15200, next: null },
        { ...snapshot.character, money: 250000 },
      ),
    )

    expect(world.career?.held?.name).toBe('The dispatch desk')
    expect(world.career?.paid).toBe(15200)
    expect(world.character?.money).toBe(250000)
    // Nothing else moved: the ladder is not the body.
    expect(world.health).toBe(before.health)
  })

  it('reads a ladder Go sent as null back as a list', () => {
    const world = applyMessage(
      hydrated(initialWorld, snapshot),
      changed({ ...onNights, held: null, paid: 0, next: null, ladder: null }),
    )
    expect(world.career?.ladder).toEqual([])
    expect(world.career?.held).toBeNull()
  })
})

/*
 * §3.14's delegated work: the one moment an active role produced that nobody's
 * rules could settle, and the two ways this client learns about it.
 *
 * Both are the server saying the whole surface. There is no message for "a
 * situation arose" separate from "a day passed", because the day passing is what
 * produced it — and no way for this client to ask for one, because §3.14's loop
 * begins "it happens without you".
 */
describe('what the job did while nobody was in it', () => {
  // The rollover, carrying what the job produced overnight. Its own copy rather
  // than the one above, because that describe is about the body and this one is
  // about the desk.
  const overnight = (situations: RolloverRan['situations']) => ({
    type: 'rollover.ran' as const,
    seq: 10,
    payload: {
      panel: { ...panel, biologicalAge: 33.6, livedToday: false },
      forecast: { days: 2, biologicalAge: [33.4, 33.2], metrics: null },
      crossed: null,
      days: 1,
      skipped: 0,
      character: { money: 120000, at: somewhere, home: somewhere, earningsTaken: 0, earningsLeft: 3 },
      situations,
    },
  })

  const answered = (situations: SituationAnswered['situations']) => ({
    type: 'situation.answered' as const,
    seq: 11,
    payload: {
      situations,
      name: 'The roster gap',
      did: 'Take the shift yourself and cover the gap.',
      came: 'The Friday pick goes out on time and nobody remembers it by Monday.',
    },
  })

  it('arrives with the rollover, because nothing else can produce one', () => {
    const before = hydrated(initialWorld, snapshot)
    expect(before.situations?.waiting).toBeNull()

    const world = applyMessage(before, overnight(theRosterGap))

    expect(world.situations?.waiting?.name).toBe('The roster gap')
    expect(world.situations?.waiting?.role).toBe('Operations manager')
    // Three things the player could do, in the content's own words — and
    // nothing anywhere saying which of them is right (§3.2).
    expect(world.situations?.waiting?.answers).toHaveLength(3)
    expect(Object.keys(world.situations?.waiting?.answers[0] ?? {}).sort()).toEqual(['does', 'key'])
  })

  it('goes away when it is answered somewhere else', () => {
    const waiting = applyMessage(hydrated(initialWorld, snapshot), overnight(theRosterGap))
    const world = applyMessage(waiting, answered({ waiting: null }))

    expect(world.situations?.waiting).toBeNull()
    // And nothing else moved: a moment being settled is not a body.
    expect(world.health).toBe(waiting.health)
    expect(world.career).toBe(waiting.career)
  })

  it('carries the next one straight through, because a waiting role stalls', () => {
    const waiting = applyMessage(hydrated(initialWorld, snapshot), overnight(theRosterGap))
    const next = { ...theRosterGap.waiting, id: 13, name: 'Someone quits' }
    const world = applyMessage(waiting, answered({ waiting: next }))

    expect(world.situations?.waiting?.id).toBe(13)
    expect(world.situations?.waiting?.name).toBe('Someone quits')
  })

  it('reads answers Go sent as null back as a list', () => {
    const world = applyMessage(
      hydrated(initialWorld, snapshot),
      answered({ waiting: { ...theRosterGap.waiting, answers: null } }),
    )
    expect(world.situations?.waiting?.answers).toEqual([])
  })

  it('is restated whole by a hydration, like every other slice the server owns', () => {
    const waiting = applyMessage(hydrated(initialWorld, snapshot), overnight(theRosterGap))
    const world = hydrated(waiting, { ...snapshot, situations: theRosterGap })

    expect(world.situations).toEqual(theRosterGap)
  })
})
