import { KeyHint } from '../chrome/Keybar'
import { drawn, Thing } from '../chrome/Things'
import { beginChallenge, chooseInChallenge } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type ChallengeRun } from '../world/state'
import { Overlay } from './Overlay'
import { spending } from './words'

/**
 * A challenge, played (§5, §3.10).
 *
 * The one screen in this client that is a *place to be* rather than a panel to
 * consult. Everything else here reports on the world; this is six minutes of
 * authored content with a committed judgement in the middle of it, and the
 * design system has its own vocabulary for exactly that — a scene head with how
 * far through you are, speech attributed to the person saying it, and options
 * that are readings to commit to rather than buttons to press.
 *
 * Three things it deliberately does not do.
 *
 * **It never says how you are doing.** There is no score on the wire and none
 * to show: §3.10's scoring is hidden, "never surfaced as a grade, never
 * confirmed as right", and the only account the player gets is the reaction the
 * content wrote for the outcome their whole passage landed in.
 *
 * **It never marks an option.** None of the readings is correct and none of
 * them is recommended. The `chosen` state the design system gives an option is
 * used for the one the player is committing to *now*, and never for a hint.
 *
 * **It does not hold the run.** Every word on screen came out of the store,
 * which the server last wrote — so closing the tab in the middle of a sentence
 * and coming back lands on the same scene, because the scene is what the server
 * checkpointed (§5.1) rather than something this component remembered.
 */
export function ChallengeScreen({ open, onClose }: { open: boolean; onClose: () => void }) {
  const challenge = useWorld((w) => w.challenge)
  const busy = useWorld(sending)

  const run = challenge?.run ?? null
  const door = challenge?.here ?? null

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title={run?.name ?? door?.name ?? 'Nothing in front of you'}
      sub={run ? run.where : (door?.door ?? '')}
    >
      {run ? (
        <Scene run={run} />
      ) : door ? (
        <div className="panel">
          <div className="panel-head">
            <span className="panel-title">{door.name}</span>
            <span className="panel-note">about six minutes · once ever</span>
          </div>
          <div className="panel-body">
            <div className="btn-row">
              <button
                className="btn primary"
                disabled={busy}
                onClick={() => void beginChallenge(door.challenge)}
              >
                {door.enter}
              </button>
            </div>
          </div>
        </div>
      ) : (
        <div className="panel">
          <div className="panel-body">
            <p className="faint tiny">Nothing is in front of you.</p>
          </div>
        </div>
      )}
    </Overlay>
  )
}

/**
 * One scene of a run: how the last thing you said landed, what is being said
 * now, and the readings you may commit to.
 *
 * The stage dots are the length rather than a mark — §3.10's "roughly 6
 * minutes" made visible so that a player deciding whether to start knows what
 * they are starting. A finished run has no question and no options: what is
 * left is the reaction, which is the whole of what they are ever told.
 */
function Scene({ run }: { run: ChallengeRun }) {
  const busy = useWorld(sending)
  /*
   * Which thing the scene is asking about, when it is asking about one.
   *
   * In the house the choices *are* the things, so each card shows itself. On
   * the pavement they are `give` and `keep` and the subject is elsewhere — the
   * first thing still in the bag, because the scene is keyed by what is left to
   * decide. Without this the screen asks "put him in her arms" with three
   * things on the shelf and no way to know which "him" is.
   */
  const deciding =
    run.holds && run.held?.length && !drawn(run.choices?.[0]?.key ?? '') ? run.held[0] : null
  const stages = Array.from({ length: Math.max(run.stages, run.stage) }, (_, i) =>
    i + 1 < run.stage ? 'done' : i + 1 === run.stage ? 'now' : '',
  )

  return (
    <>
      {/* A dialogue's reaction to what you committed to (§3.10). A bag
          challenge has none: the object's backstory said on the pavement is a
          sentence about a fridge nobody can see. */}
      {run.taken && !run.holds && <p className="speech aside">{run.taken}</p>}

      <div className="scene-head">
        <span className="stage-dots">
          {stages.map((state, i) => (
            <i className={state} key={i} />
          ))}
        </span>
      </div>

      {/* The bag, for a mechanic that has one (§5.2's "matching client scene
          module", as the one piece of chrome `salvage` needs that
          `dialogue_branch` does not). Not shown once the run is done: what you
          walked out with stops being a running total and becomes the story. */}
      {!run.done && run.holds ? (
        <Bag held={run.held ?? []} holds={run.holds} deciding={deciding?.key ?? null} />
      ) : null}

      {/* How it went (§5.1). Authored per outcome band and the only account
          the player ever gets of a finished run — the score, the band and the
          weights all stay hidden, so a run that ended and said nothing left
          six minutes of choices looking like they went nowhere. What is
          deliberately not here is the band's name: the words are the whole
          verdict, and a label beside them would grade the run. */}
      {run.done && run.closing ? (
        <div className="panel">
          <div className="panel-body">
            <p className="speech mt0">{run.closing}</p>
            {run.earned ? (
              <p className="faint tiny">You came out of it {spending(run.earned)} up.</p>
            ) : null}
          </div>
        </div>
      ) : null}

      {run.done ? null : (
        <>
          {/* A run with a bag has already said all of this in the room: the
              things are in front of you and the bag is under them. Repeating it
              as three paragraphs is the reading exercise this challenge is not
              — so only the one line somebody actually said survives, and the
              narration goes. A dialogue has nothing but its beats and keeps
              all of them. */}
          <div className="panel">
            <div className="panel-body">
              {/* Keyed by what was said and who said it, not by position. A
                  run with a bag filters the narration out of this list, so
                  position is exactly the thing that moves: the same index is
                  one beat before the filter and a different one after, and
                  React would keep the old paragraph and swap the text under
                  it. The pair is the beat's identity — `who` is empty for
                  narration, which makes one expression serve both branches. */}
              {(run.holds ? (run.beats ?? []).filter((beat) => beat.who) : (run.beats ?? [])).map((beat) =>
                beat.who ? (
                  <div className="reply" key={`${beat.who}:${beat.line}`}>
                    <p className="speech mt0">{beat.line}</p>
                    <p className="reply-who">{beat.who}</p>
                  </div>
                ) : (
                  <p className="speech" key={`${beat.who}:${beat.line}`}>
                    {beat.line}
                  </p>
                ),
              )}
            </div>
          </div>

          <div className="ask">
            <span className="ask-n">●</span>
            <span>
              <p className="ask-text">{run.ask}</p>
            </span>
          </div>

          {/* A run with a bag takes things in the *room* — walk up and press O,
              or click the thing. A list of sentences beside it would be a
              second way to do the same act, and the one that is not a game.
              The pavement is different: keep-or-give is a decision about
              something already in your hands, not a thing to reach for. */}
          <div className="commit">
            {(run.holds && !deciding ? [] : (run.choices ?? [])).map((choice) => (
              <div
                className="option"
                key={choice.key}
                onClick={() => {
                  if (!busy) void chooseInChallenge(run.challenge, run.scene, choice.key)
                }}
              >
                {/* A mechanic whose choices are *objects* shows them. One whose
                    choices are readings of a situation has nothing to draw, and
                    a picture beside a sentence somebody is weighing would be
                    decoration pretending to be information. `holds` is the
                    honest test: a run with a bag is a run about things. */}
                {run.holds && (drawn(choice.key) || deciding) ? (
                  <div className="option-thing">
                    <Thing item={drawn(choice.key) ? choice.key : (deciding?.key ?? '')} size={34} />
                  </div>
                ) : null}
                <div>
                  <div className="option-text">{choice.text}</div>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </>
  )
}

/**
 * What you are carrying, and what is still free.
 *
 * The whole of the `salvage` mechanic's own chrome, and it earns its place by
 * being the one thing the words cannot say. A player choosing their third item
 * has to know what the first two were and that this is the last one — a
 * sentence in the narration would say it once, at the top, and be scrolled past
 * by the time it mattered.
 *
 * Empty slots are drawn rather than implied. "Two of three" is a number to
 * read; two full slots and one empty one is a fact you can see, and the
 * emptiness is the pressure the whole scene is built on.
 *
 * There is nothing here about what any of it is worth. What is carried is a
 * list of things the player chose, in the content's own words — the scoring is
 * hidden and stays hidden (§3.10), and a bag that showed a running anything
 * would be the grade arriving by the one route still open.
 */
function Bag({
  held,
  holds,
  deciding,
}: {
  held: { key: string; name: string }[]
  holds: number
  deciding?: string | null
}) {
  const slots = Array.from({ length: holds }, (_, i) => held[i] ?? null)
  const free = holds - held.length

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">In the bag</span>
        <span className="panel-note">
          {free > 0 ? `${free} of ${holds} still free` : `full — ${holds} of ${holds}`}
        </span>
      </div>
      <div className="panel-body">
        <div className="bag-slots">
          {slots.map((what, i) => (
            <div
              className={
                what
                  ? `bag-slot filled${what.key === deciding ? ' deciding' : ''}`
                  : 'bag-slot'
              }
              key={what?.key ?? `free-${i}`}
            >
              {what ? (
                <>
                  <Thing item={what.key} size={30} />
                  <span className="bag-name">{what.name}</span>
                </>
              ) : (
                <span className="bag-empty">empty</span>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

/**
 * The key that opens a challenge (§5), and it is only there when one is.
 *
 * A challenge is a six-minute endeavour played once ever, so there is nothing
 * to look at between them — and a key in the bar that opened an empty panel
 * would be a menu entry for a thing that does not exist. When a run is part way
 * through, the hint says so, because §5.1's whole point is that abandoning
 * partway is expected and coming back has to be obvious.
 */
export function ChallengeHint() {
  const challenge = useWorld((w) => w.challenge)
  if (!challenge) return null

  if (challenge.run && !challenge.run.done) {
    return <KeyHint keys={['C']}>{`carry on — ${challenge.run.name.toLowerCase()}`}</KeyHint>
  }
  if (challenge.here) {
    return <KeyHint keys={['C']}>{challenge.here.name.toLowerCase()}</KeyHint>
  }
  return null
}
