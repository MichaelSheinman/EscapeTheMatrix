import { KeyHint } from '../chrome/Keybar'
import { beginChallenge, chooseInChallenge } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type ChallengeRun } from '../world/state'
import { Overlay } from './Overlay'

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
      sub={
        run
          ? run.where
          : door
            ? 'It is happening where you are standing. Nothing makes you join in.'
            : 'These find you where you are, and mostly while you are walking. There is no list of them.'
      }
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
            <p className="note first">
              You can stand there and listen, or you can go up your own stairs. It will not be
              here twice.
            </p>
            <div className="btn-row">
              <button
                className="btn primary"
                disabled={busy}
                onClick={() => void beginChallenge(door.challenge)}
              >
                Stop and listen
              </button>
            </div>
          </div>
        </div>
      ) : (
        <div className="panel">
          <div className="panel-body">
            <p className="note first">
              Nothing is in front of you. The world decides when something is, and walking is
              where most of it happens.
            </p>
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
  const stages = Array.from({ length: Math.max(run.stages, run.stage) }, (_, i) =>
    i + 1 < run.stage ? 'done' : i + 1 === run.stage ? 'now' : '',
  )

  return (
    <>
      <div className="scene-head">
        <span className="tag amber">{run.name}</span>
        <span className="stage-dots">
          {stages.map((state, i) => (
            <i className={state} key={i} />
          ))}
        </span>
        <span className="faint tiny">
          {run.done ? 'that was all of it' : `${run.where.toLowerCase()} · once ever`}
        </span>
      </div>

      {run.taken && <p className="speech aside">{run.taken}</p>}

      {run.done ? (
        <div className="panel">
          <div className="panel-body">
            <p className="speech">{run.closing}</p>
          </div>
        </div>
      ) : (
        <>
          <div className="panel">
            <div className="panel-body">
              {run.beats.map((beat, i) =>
                beat.who ? (
                  <div className="reply" key={i}>
                    <p className="speech mt0">{beat.line}</p>
                    <p className="reply-who">{beat.who}</p>
                  </div>
                ) : (
                  <p className="speech" key={i}>
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
              <p className="ask-note">
                None of these is the right one. Each of them goes somewhere else, and you will not
                be told which was which.
              </p>
            </span>
          </div>

          <div className="commit">
            {run.choices.map((choice) => (
              <div
                className="option"
                key={choice.key}
                onClick={() => {
                  if (!busy) void chooseInChallenge(run.challenge, run.scene, choice.key)
                }}
              >
                <div className="option-lead">Commit to this</div>
                <div className="option-text">{choice.text}</div>
              </div>
            ))}
          </div>
        </>
      )}
    </>
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
