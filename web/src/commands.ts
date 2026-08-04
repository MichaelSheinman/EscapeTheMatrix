/*
 * The command client.
 *
 * technical.md §8: commands go over REST, one endpoint each, addressed by name.
 * This module is the only place in the client that knows that — components call
 * `command(...)` and never build a URL.
 *
 * Idempotency keys and the typed command registry arrive with the command
 * layer ticket. The seam is here so that adding them changes this file only.
 */

/** What the walking-skeleton probe returns. Every field came from Postgres. */
export type ProbeResult = {
  probes: number
  notedAt: string
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
}

/** Send a command by name and decode its result. */
export async function command<T>(name: string): Promise<T> {
  const res = await fetch(`/api/commands/${name}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
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

  return (await res.json()) as T
}
