/*
 * The session client.
 *
 * technical.md §9: account required up front, no anonymous play, sessions
 * server-side in Postgres behind an httpOnly cookie. The last part is why this
 * module is so thin — the credential is a cookie the browser attaches on its
 * own and no script here can read, so there is no token to store, refresh, or
 * accidentally put in localStorage.
 *
 * What the client holds is the *answer*: who the server says we are. It asks
 * once on load, which is what makes a signed-in identity survive a reload.
 */

/** A signed-in player, as the server describes them. */
export type Player = {
  id: string
  name: string
  email: string
  avatarUrl: string
}

/** Where the browser goes to sign in. A navigation, not a fetch. */
export const signInURL = '/api/auth/google/start'

/**
 * Who the server thinks is asking, or null when nobody is.
 *
 * Being signed out is an answer rather than a failure, so it is a 200 with no
 * player — the app has a first screen to show either way.
 */
export async function currentPlayer(): Promise<Player | null> {
  const res = await fetch('/api/auth/session', {
    headers: { Accept: 'application/json' },
    // The answer is per-player and must never come from a cache.
    cache: 'no-store',
  })
  if (!res.ok) throw new Error(`the session could not be read (${res.status})`)

  const body = (await res.json()) as { player: Player | null }
  return body.player
}

/**
 * End the session.
 *
 * The cookie going away is the visible half; the half that matters is the
 * server marking the session revoked, so the same cookie replayed from anywhere
 * is worth nothing.
 */
export async function signOut(): Promise<void> {
  const res = await fetch('/api/auth/signout', { method: 'POST' })
  if (!res.ok) throw new Error(`signing out failed (${res.status})`)
}

/**
 * Why a sign-in attempt did not finish, if it just didn't.
 *
 * The callback is a top-level navigation, so the server cannot answer it with
 * JSON the app would see — it sends the browser back to `/` with a reason in
 * the query string instead. Reading it clears it, so a reload does not re-show
 * a complaint about something that already happened.
 */
export function takeSignInFailure(): string | null {
  const reason = new URLSearchParams(window.location.search).get('signin')
  if (!reason) return null

  window.history.replaceState({}, '', window.location.pathname)
  switch (reason) {
    case 'declined':
      return 'Google did not complete the sign-in.'
    case 'expired':
      return 'That sign-in took too long. Try again.'
    default:
      return 'Something went wrong signing in. Try again.'
  }
}
