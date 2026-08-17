/*
 * The server's numbers and instants, written down in English.
 *
 * Everything here is presentation over a value the server sent — the client
 * still knows no rule about what any number means. Small on purpose: a term
 * belongs here only when more than one screen says it, and the state→English
 * vocabularies (what became of an answer, how a moment left a condition) stay
 * with the one screen that speaks them.
 */

import type { BoardId } from '../world/state'

/**
 * Cents, as money reads.
 *
 * The server sends an integer number of cents and this divides it by a hundred
 * to write it down — the only arithmetic the shell does on money.
 */
export function spending(cents: number): string {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
  }).format(cents / 100)
}

/**
 * A revealed number, in whatever the server said to read it in.
 *
 * Empty for a sealed value — and there is nothing to hide, because it arrived
 * null. The formatting is chosen by a string the server sent, so this client
 * still knows no rule about what any factor means. One function for the karma
 * slots and the board rungs both, because it is one rule.
 */
export function byUnit(reads: BoardId['reads'], value: number | null): string {
  if (value === null) return ''
  return reads === 'money' ? spending(value) : value.toFixed(0)
}

/**
 * Cents, as money reads in a space the width of a coin.
 *
 * Rounded to three significant figures and suffixed, which is how a balance
 * fits inside an orb on the heads-up display. Everywhere there is room for the
 * whole number, [spending] is the one to use — this drops digits.
 */
export function briefly(cents: number): string {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    notation: 'compact',
    maximumSignificantDigits: 3,
  }).format(cents / 100)
}

/** 1 → 1st. Presentation, and English's problem rather than the server's. */
export function ordinal(n: number): string {
  const tens = n % 100
  if (tens >= 11 && tens <= 13) return `${n}th`
  return `${n}${['th', 'st', 'nd', 'rd'][n % 10] ?? 'th'}`
}

/** An RFC 3339 instant, as a local time somebody can read. */
export function clockOf(at: string): string {
  const when = new Date(at)
  if (Number.isNaN(when.getTime())) return at
  return when.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

/** An RFC 3339 instant, as a date somebody can read. */
export function dayOf(at: string): string {
  const when = new Date(at)
  if (Number.isNaN(when.getTime())) return at
  return when.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
