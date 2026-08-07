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

/** Payload of the `probe.recorded` message. */
export type ProbeRecorded = {
  probes: number
  label: string
  notedAt: string
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
  | { type: 'probe.recorded'; seq: number; payload: ProbeRecorded }
  | { type: 'welcome'; seq: number; payload: Welcome }

/** The name of any server message. */
export type ServerMessageType = ServerMessage['type']
