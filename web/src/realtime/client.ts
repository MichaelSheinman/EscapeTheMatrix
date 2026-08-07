/*
 * The socket client.
 *
 * technical.md §8: commands go up over REST, and everything the server decides
 * on its own comes down one WebSocket. This module is the only place in the
 * client that knows the socket exists — callers hand it a function and receive
 * typed messages.
 *
 * The connection dropping is the normal case, not the failure case. Laptops
 * sleep, wifi flaps, deploys roll. So the only interesting thing here is the
 * cursor: the sequence number of the last message processed, sent back on
 * reconnect, which is what makes the server replay what was missed instead of
 * the world silently skipping.
 *
 * What it deliberately does not do is buffer, retry, or send. There is no
 * client→server message at all — resume is a query parameter on the handshake —
 * so a reconnect is caught up by the time it is open.
 */

import type { ServerMessage } from './messages.gen'

export type { ServerMessage } from './messages.gen'

/** What the connection is doing, for a client that wants to say so on screen. */
export type Status = 'connecting' | 'live' | 'offline'

export type Options = {
  /** Called for every message except the connection's own welcome. */
  onMessage: (message: ServerMessage) => void
  onStatus?: (status: Status) => void
  /**
   * The server had already swept the messages this tab asked to resume from,
   * so the patches that follow cannot be trusted to land on the right state.
   * The answer is to reload from REST — which is what opening the page does.
   */
  onGap?: () => void
}

/** A live subscription. Closing it stops the reconnect loop for good. */
export type Stream = { close: () => void }

/** How long to wait before the nth reconnect attempt, in milliseconds. */
const backoff = (attempt: number) =>
  Math.min(500 * 2 ** attempt, 15_000) * (0.5 + Math.random() / 2)

const SESSION_KEY = 'etm.session'

/**
 * This tab's session id.
 *
 * Per tab rather than per player, because that is what makes two tabs work: a
 * message caused by a command from this tab is skipped by this tab's socket and
 * delivered to the other one. It lives in `sessionStorage`, so it survives a
 * reload and a reconnect — a new id after every drop would hand this tab back
 * its own actions as though somebody else had done them — and dies with the
 * tab, which is exactly its lifetime.
 */
export function sessionID(): string {
  const held = sessionStorage.getItem(SESSION_KEY)
  if (held) return held

  const id = crypto.randomUUID
    ? crypto.randomUUID()
    : Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) =>
        b.toString(16).padStart(2, '0'),
      ).join('')
  sessionStorage.setItem(SESSION_KEY, id)
  return id
}

/**
 * Open the socket and keep it open.
 *
 * The first connection asks for nothing that happened before it: the page has
 * just loaded its state over REST, so replaying the last hour on top of it
 * would apply the same things twice. Every reconnect after that asks for
 * everything past the last sequence number this tab processed.
 */
export function connect({ onMessage, onStatus, onGap }: Options): Stream {
  const session = sessionID()

  let cursor: number | null = null
  let socket: WebSocket | null = null
  let attempt = 0
  let retry: ReturnType<typeof setTimeout> | undefined
  let closed = false

  const status = (s: Status) => onStatus?.(s)

  function url(): string {
    const u = new URL('/api/socket', window.location.href)
    u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:'
    u.searchParams.set('session', session)
    // Absent on the first connection and present on every reconnect. The
    // server reads the difference as "from now on" versus "from here".
    if (cursor !== null) u.searchParams.set('since', String(cursor))
    return u.toString()
  }

  function open() {
    if (closed) return
    status('connecting')

    const ws = new WebSocket(url())
    socket = ws

    ws.onmessage = (e) => {
      let message: ServerMessage
      try {
        message = JSON.parse(e.data as string) as ServerMessage
      } catch {
        // A frame this build cannot parse is a server newer than this tab.
        // Dropping it is the only safe reading, and the cursor deliberately
        // does not move past it.
        return
      }

      if (message.type === 'welcome') {
        // The connection is only live once the server has said where it put
        // us, which is also when the backoff is worth resetting.
        attempt = 0
        status('live')

        // A fresh connection adopts the server's head as its starting point.
        // A resumed one keeps its own — the replay that follows this message
        // is exactly what it is missing — unless the server says the replay is
        // incomplete, in which case there is nothing to do but reload.
        if (cursor === null || message.payload.gap) cursor = message.payload.head
        if (message.payload.gap) onGap?.()
        return
      }

      // Seq 0 is connection control and never advances the cursor; anything
      // else is the ordered stream, and the cursor is what the next reconnect
      // resumes from. It moves *after* the message is handed over, so a
      // handler that throws does not skip it on the way back.
      onMessage(message)
      if (message.seq > 0) cursor = message.seq
    }

    ws.onclose = () => {
      if (socket === ws) socket = null
      if (closed) return
      status('offline')
      retry = setTimeout(open, backoff(attempt++))
    }

    // An error is always followed by a close, which is where the retry lives.
    ws.onerror = () => ws.close()
  }

  /**
   * Try again now.
   *
   * A laptop that just woke has a socket the browser has not noticed is dead
   * and a backoff timer that may be fifteen seconds from firing. Both of the
   * events below mean the environment changed, so the polite wait is over.
   */
  function reconnectNow() {
    if (closed || socket) return
    clearTimeout(retry)
    attempt = 0
    open()
  }

  window.addEventListener('online', reconnectNow)
  const onVisible = () => {
    if (document.visibilityState === 'visible') reconnectNow()
  }
  document.addEventListener('visibilitychange', onVisible)

  open()

  return {
    close() {
      closed = true
      clearTimeout(retry)
      window.removeEventListener('online', reconnectNow)
      document.removeEventListener('visibilitychange', onVisible)
      socket?.close()
      socket = null
    },
  }
}
