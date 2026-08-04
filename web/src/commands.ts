/*
 * The command client.
 *
 * technical.md §8: commands go over REST, one endpoint each, addressed by name,
 * carrying an `Idempotency-Key`. This module is the only place in the client
 * that knows any of that — components call `command(...)` and never build a URL
 * or think about a header.
 *
 * The key is what makes a retry safe. Generate it once per *intent* and reuse
 * it for every delivery of that intent: the retry after a timeout, the resend
 * after a reload. A new key means a new action, and the server will apply it.
 */

/** What the walking-skeleton probe takes and returns. */
export type ProbeInput = { label?: string }

export type ProbeResult = {
  probes: number
  notedAt: string
  label: string
  database: string
  schema: number
}

/** A command that the server rejected or that never arrived. */
export class CommandError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }

  /**
   * The request was wrong and will be wrong again (422 · 400). Status codes are
   * the protocol, so this is decidable without reading the message.
   */
  get isPermanent(): boolean {
    return this.status === 422 || this.status === 400
  }

  /** The world was in the way (409). The same command may work later. */
  get isConflict(): boolean {
    return this.status === 409
  }

  /**
   * There is no live session (401). Every command goes through one middleware
   * (§8), so this is the one refusal that can arrive from any of them — the
   * session expired, or was signed out on another device — and it is the app's
   * cue to go back to the sign-in screen rather than to retry.
   */
  get isSignedOut(): boolean {
    return this.status === 401
  }
}

/** One delivery of a command, and what the server said about it. */
export type Sent<T> = {
  result: T
  /** The key it was sent under — hold on to it to retry the same intent. */
  key: string
  /** The server answered from a record of an earlier delivery. */
  replayed: boolean
}

/**
 * A fresh idempotency key: one per intent.
 *
 * `crypto.randomUUID` needs a secure context, which localhost and production
 * both are — the fallback is for the one case in between, a dev build opened
 * over plain http on a LAN address from another device.
 */
export function newKey(): string {
  if (crypto.randomUUID) return crypto.randomUUID()
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

/** Send a command by name and decode its result. */
export async function command<T>(
  name: string,
  input: unknown = {},
  key: string = newKey(),
): Promise<Sent<T>> {
  const res = await fetch(`/api/commands/${name}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': key,
    },
    body: JSON.stringify(input),
  })

  if (!res.ok) {
    // The server answers failures in JSON too, but a proxy or a crash can
    // return HTML — so a decode failure must not mask the real status.
    const detail = await res
      .json()
      .then((b: { error?: string }) => b.error)
      .catch(() => undefined)
    throw new CommandError(res.status, detail ?? `command ${name} failed`)
  }

  return {
    result: (await res.json()) as T,
    key,
    replayed: res.headers.get('Idempotent-Replay') === 'true',
  }
}
