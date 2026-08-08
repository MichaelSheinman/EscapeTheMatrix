import { useEffect, useState } from 'react'

import { KeyHint } from '../chrome/Keybar'
import { endorse, loadBoardPage, loadBoards, passOnOffer } from '../world/actions'
import { useWorld } from '../world/react'
import type { BoardPage } from '../world/state'
import { Overlay } from './Overlay'
import { byUnit, clockOf, ordinal } from './words'

/**
 * The leaderboards (§3.5).
 *
 * §3.5 asks for a **distributed** board: one overall board for the composite,
 * plus a separate board for each individual factor — and the three hidden
 * factors get boards too, because a ranking is a *position* rather than a
 * number, and showing one discloses nothing about the formula.
 *
 * So three of the tabs below have no name. They are not placeholders and they
 * are not disabled: they rank real players by something real, and finding out
 * what is the question §3.5 is asking. Nothing on this screen could reveal it if
 * it wanted to — a sealed board's rows arrive with `value: null`, and the field
 * a number would be in is empty on the wire rather than filtered here.
 *
 * The list of other people is the one thing in this client that is fetched
 * rather than pushed (§10.2's read-only paginated list). Where the *player*
 * stands is in the store, because the socket moves it when somebody else plays.
 */
export function BoardsScreen({ open, onClose }: { open: boolean; onClose: () => void }) {
  const boards = useWorld((w) => w.boards)
  const [key, setKey] = useState('karma')
  const [page, setPage] = useState(1)
  const [showing, setShowing] = useState<BoardPage | null>(null)

  // Opening the screen is what loads it. A leaderboard nobody is looking at is
  // a fetch nobody asked for, and the boot snapshot deliberately omits it.
  useEffect(() => {
    if (open) void loadBoards()
  }, [open])

  // The page is re-read whenever the board, the page, or the *snapshot* changes
  // — the last of those is how a rank that moved while this was open arrives:
  // the socket patches `refreshedAt`, and the list underneath it is stale.
  const refreshedAt = boards?.refreshedAt ?? ''
  useEffect(() => {
    if (!open) return
    let live = true
    void loadBoardPage(key, page).then((got) => {
      if (live) setShowing(got)
    })
    return () => {
      live = false
    }
  }, [open, key, page, refreshedAt])

  function choose(next: string) {
    setKey(next)
    setPage(1)
  }

  const standing = boards?.boards.find((p) => p.board.key === key) ?? null

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="The boards"
      sub="One board for the whole of it, and one for each of the things it is made of. Three of
            those have no name. They rank you all the same."
    >
      {/* The one thing on this screen that is about a *person* rather than
          about a position. It sits above the boards because §3.5 pairs them
          — the boards are how you see that other people exist, and this is
          the only thing you may do about one of them. */}
      <Somebody />

      {!boards ? (
        <p className="note first">Reading the boards…</p>
      ) : (
        <>
          <div className="tabs">
            {boards.boards.map((placing, i) => (
              <a
                key={placing.board.key}
                className={placing.board.key === key ? 'active' : ''}
                tabIndex={0}
                onClick={() => choose(placing.board.key)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') choose(placing.board.key)
                }}
              >
                {/* A sealed board is keyed by position on the wire, and the
                    tab says so — the same "there is something here" a
                    locked slot says on the standing screen. */}
                {placing.board.shown ? placing.board.name : `? ? ? ? ? (${i})`}
              </a>
            ))}
          </div>

          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">
                {standing?.board.shown ? standing.board.name : 'Something'}
              </span>
              <span className="panel-note">
                {boards.players === 1 ? '1 player' : `${boards.players} players`}
                {boards.refreshedAt ? ` · as of ${clockOf(boards.refreshedAt)}` : ''}
              </span>
            </div>

            <div className="panel-body">
              <p className="note first">{standing?.board.note ?? ''}</p>

              {standing && standing.rank > 0 && (
                <div className="env-chip">
                  <span className="env-label">you are</span>
                  <span className="env-name">
                    {ordinal(standing.rank)} of {standing.of}
                  </span>
                </div>
              )}

              {!showing || showing.rows.length === 0 ? (
                <p className="note last">
                  Nobody is on this board yet. It is rebuilt in the background rather than
                  worked out when you look, so it will fill in on its own.
                </p>
              ) : (
                <>
                  {showing.rows.map((row) => (
                    <div className={`board${row.you ? ' you' : ''}`} key={row.rank}>
                      <span className="rank">{row.rank}</span>
                      <span>
                        <b>{row.name}</b>
                        <i>{row.you ? 'you' : ''}</i>
                      </span>
                      <span className="metric-value">
                        {byUnit(showing.board.reads, row.value)}
                      </span>
                    </div>
                  ))}

                  {showing.pages > 1 && (
                    <div className="btn-row">
                      <button
                        className="btn small"
                        onClick={() => setPage(showing.page - 1)}
                        disabled={showing.page <= 1}
                      >
                        back
                      </button>
                      <span className="faint tiny">
                        page {showing.page} of {showing.pages}
                      </span>
                      <button
                        className="btn small"
                        onClick={() => setPage(showing.page + 1)}
                        disabled={showing.page >= showing.pages}
                      >
                        next
                      </button>
                    </div>
                  )}
                </>
              )}

              <p className="faint tiny">
                Rebuilt by the server on a schedule, not when you open this. Which is why it
                says when it was taken, and why your own place can move while you are reading
                it — somebody else played.
              </p>
            </div>
          </div>
        </>
      )}
    </Overlay>
  )
}

/**
 * Peer endorsement (§3.5).
 *
 * §3.5 asks for "an occasional 'I like this user' button players can press. Rate
 * it, don't make it always-available", and the shape that satisfies the second
 * half is why this panel is usually empty: **the game names one person, now and
 * then, and there is nothing here to type.** No search, no list of players, no
 * way to say who — because a control that let you choose would be an
 * always-available button with a rate limiter bolted to it.
 *
 * So the empty state is the ordinary one and it has to read as such, which is
 * what `opens` is for: a button that vanishes without saying when it comes back
 * is indistinguishable from a screen that is broken.
 *
 * The bottom half is the part the design is actually after. Being told that
 * somebody said they like you is the only human signal in V1 — everything else
 * player-to-player needs both of you present and is V2 (§3.7). Note what it does
 * *not* say: what it is worth, or that it is worth anything. That question is
 * §3.5's hook and this screen is not the place it gets answered.
 */
function Somebody() {
  const endorsement = useWorld((w) => w.endorsement)
  if (!endorsement) return null

  const offer = endorsement.offer

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">Somebody to like</span>
        <span className="panel-note">
          {endorsement.given} of {endorsement.cap} given this week
        </span>
      </div>

      <div className="panel-body">
        {offer ? (
          <>
            <div className="ask">
              <span className="ask-n">♥</span>
              <span>
                <p className="ask-text">
                  Do you like <b>{offer.who.name}</b>?
                </p>
                <p className="ask-note">
                  A real player, picked by the game. They will never know you were asked, and you
                  will not be asked about them again. Open until {clockOf(offer.until)}.
                </p>
              </span>
            </div>

            <div className="btn-row">
              <button className="btn" onClick={() => void endorse(offer.id)}>
                I like this player
              </button>
              <button className="btn small" onClick={() => void passOnOffer(offer.id)}>
                not this time
              </button>
            </div>
          </>
        ) : (
          <p className="note first">
            Nobody right now.{' '}
            {endorsement.opens
              ? `The game will pick somebody again after ${clockOf(endorsement.opens)}.`
              : 'Somebody will turn up.'}
          </p>
        )}

        {endorsement.received > 0 && (
          <p className="note last">
            {endorsement.received === 1
              ? 'One player has said they like you'
              : `${endorsement.received} players have said they like you`}
            {endorsement.liked.length > 0 ? `: ${endorsement.liked.map((p) => p.name).join(', ')}.` : '.'}
          </p>
        )}
      </div>
    </div>
  )
}

/**
 * The key that opens the boards, which is also where an offer waits (§3.5).
 *
 * An offer stands for a matter of hours and then lapses, so a player who never
 * opens that screen would simply never meet the feature. The hint changes rather
 * than a badge appearing beside it: the shell has no rail and no notification
 * area, and the key bar saying something different *is* how this world tells you
 * something is there.
 */
export function BoardsHint() {
  const waiting = useWorld((w) => w.endorsement?.offer != null)
  return <KeyHint keys={['L']}>{waiting ? 'the boards — somebody to like' : 'the boards'}</KeyHint>
}
