// Code generated from the Go message definitions in internal/realtime.
// DO NOT EDIT — change the Go struct and run `make generate`.
//
// technical.md §8: every server→client message is {type, seq, payload}, and
// these types are generated rather than hand-maintained because two lists of
// the same protocol drift and you find out in production.

/** Payload of the `character.earned` message. */
export type CharacterEarned = {
  money: number
  earned: number
  earningsTaken: number
  earningsLeft: number
}

/** Payload of the `character.moved` message. */
export type CharacterMoved = {
  at: {
    key: string
    name: string
    note: string
  }
}

/** Payload of the `health.bought` message. */
export type HealthBought = {
  panel: {
    biologicalAge: number
    livedToday: boolean
    tiers: ({
      key: string
      name: string
      buys: string
      price: number
      owned: boolean
      forSale: boolean
    })[] | null
    metrics: ({
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
      history: number[] | null
    })[] | null
  }
  tier: {
    key: string
    name: string
    buys: string
    price: number
    owned: boolean
    forSale: boolean
  }
  spent: number
  money: number
}

/** Payload of the `health.lived` message. */
export type HealthLived = {
  panel: {
    biologicalAge: number
    livedToday: boolean
    tiers: ({
      key: string
      name: string
      buys: string
      price: number
      owned: boolean
      forSale: boolean
    })[] | null
    metrics: ({
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
      history: number[] | null
    })[] | null
  }
  crossed: ({
    metric: string
    from: string
    to: string
    worse: boolean
  })[] | null
}

/** Payload of the `probe.recorded` message. */
export type ProbeRecorded = {
  probes: number
  label: string
  notedAt: string
}

/** Payload of the `rollover.ran` message. */
export type RolloverRan = {
  panel: {
    biologicalAge: number
    livedToday: boolean
    tiers: ({
      key: string
      name: string
      buys: string
      price: number
      owned: boolean
      forSale: boolean
    })[] | null
    metrics: ({
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
      history: number[] | null
    })[] | null
  }
  forecast: {
    days: number
    biologicalAge: number[] | null
    metrics: ({
      key: string
      values: number[] | null
    })[] | null
  }
  crossed: ({
    metric: string
    from: string
    to: string
    worse: boolean
  })[] | null
  days: number
  skipped: number
}

/** Payload of the `routine.set` message. */
export type RoutineSet = {
  routines: {
    set: boolean
    routines: ({
      key: string
      name: string
      asks: string
      learn: string
      unlocked: boolean
      chosen: string
      options: ({
        key: string
        says: string
        costs: string
        chosen: boolean
      })[] | null
    })[] | null
  }
  forecast: {
    days: number
    biologicalAge: number[] | null
    metrics: ({
      key: string
      values: number[] | null
    })[] | null
  }
}

/** Payload of the `welcome` message. */
export type Welcome = {
  session: string
  since: number
  head: number
  gap: boolean
}

/** Every message the server can send, discriminated by `type`. */
export type ServerMessage =
  | { type: 'character.earned'; seq: number; payload: CharacterEarned }
  | { type: 'character.moved'; seq: number; payload: CharacterMoved }
  | { type: 'health.bought'; seq: number; payload: HealthBought }
  | { type: 'health.lived'; seq: number; payload: HealthLived }
  | { type: 'probe.recorded'; seq: number; payload: ProbeRecorded }
  | { type: 'rollover.ran'; seq: number; payload: RolloverRan }
  | { type: 'routine.set'; seq: number; payload: RoutineSet }
  | { type: 'welcome'; seq: number; payload: Welcome }

/** The name of any server message. */
export type ServerMessageType = ServerMessage['type']
