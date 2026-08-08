import { useEffect, useState } from 'react'

import { KeyHint } from '../chrome/Keybar'
import { clearRule, composeRule, loadPlaybookLog } from '../world/actions'
import { district, placeIn } from '../world/city'
import { useWorld } from '../world/react'
import {
  sending,
  type Fire,
  type PlaybookLog,
  type PlaybookQuestion,
  type PlaybookRef,
  type PlaybookRule,
  type PlaybookSet,
  type PlaybookTerm,
  type PlaybookTrend,
  type World,
} from '../world/state'
import { Overlay } from './Overlay'
import { clockOf, dayOf } from './words'

/* ── the playbook (§3.14, §3.15) ───────────────────────────────────────── */

/**
 * Where a parameterised term's number goes in the sentence the server sent.
 *
 * The one string constant this client shares with the vocabulary, and it is
 * shared rather than resolved server-side on purpose: the builder puts a menu
 * *inside* the sentence — "they are paying me [above $60]" — and a finished
 * string would have to be taken apart again to do it.
 */
const HOLE = '{value}'

/** Which part of the sentence is being edited. */
type Part = 'c0' | 'c1' | 'then' | 'else'

/** One line of a menu: what it says, and what pressing it does. */
type Choice = { label: string; act: () => void }

/**
 * The set in force where the player lives.
 *
 * Found by the key the *server* stated (`karma.where`) rather than by anything
 * derived from where the character is standing. An environment is a population
 * and a population is not a place, so there is nothing on this client that
 * could work out which one is in force; and if there were, it would say
 * something different from the matcher the moment the player walked downtown.
 * One selector, because the chip and the hint answering it differently would
 * be two opinions about a fact the server owns.
 */
const activeSet = (w: World): PlaybookSet | null => {
  const where = w.karma?.where ?? ''
  return w.playbook?.sets.find((s) => s.environment === where) ?? null
}

/**
 * Which rule set is in force where you live (§3.15), floated beside the place.
 *
 * The population and the count are both the server's: the environment comes
 * from `karma.where` — never derived from where the character is standing —
 * and the chip counts the answers actually running there, so committing one in
 * the playbook is visible from the street a second later.
 */
export function RuleSet() {
  const among = useWorld((w) => w.karma?.among ?? '')
  const set = useWorld(activeSet)
  if (!among || !set) return null

  const held = set.rules.filter((r) => r.inForce).length
  return (
    <div className="env-chip">
      <span className="env-label">Rule set in force</span>
      <span className="env-name">{among}</span>
      <span className={`tag${held < 2 ? ' amber' : ''}`}>{held} answered</span>
    </div>
  )
}

/**
 * The key that opens the playbook, saying what is wrong with it where you are.
 *
 * The same device the boards hint uses, and for the same reason: there is no
 * rail and no notification area, so the key bar reading differently *is* how
 * this world tells you something wants you. What it reports is scoped to the
 * set for the people the player lives among, because that is the only set
 * running right now — §3.15's point is that the answers you wrote for another
 * population are not doing anything until you are living among them.
 */
export function PlaybookHint() {
  const set = useWorld(activeSet)

  const clashing = set?.conflicts.some((c) => c.always) ?? false
  const open = set ? set.rules.filter((r) => !r.inForce).length : 0

  return (
    <KeyHint keys={['P']}>
      {clashing
        ? 'your playbook — two answers disagree here'
        : open > 0
          ? `your playbook — ${open} unanswered here`
          : 'your playbook'}
    </KeyHint>
  )
}

/**
 * The playbook: the rules you have set for your own judgement (§3.14).
 *
 * Two things about this screen are the design rather than the styling.
 *
 * **The editor is the sentence.** There is no form and there is nothing to
 * press afterwards: every part of the rule opens a short list of what that part
 * could be, in place, under the word — so the rule stays readable while it is
 * being changed, and the change *is* the change. §3.14 gives the reason a save
 * button would be dishonest: the world is running behind this panel either way,
 * so a commit step would only mean something if the player could sit and think
 * in a paused world.
 *
 * **It is a set of sets, and the tabs say so first.** §3.15's whole thesis is
 * that the right answer depends on where you are, and a screen that opened on
 * one list of rules would teach the opposite before the player read a word. So
 * the environments come before the rules, every one of them exists from the
 * start — including the districts nobody has visited — and the one you are
 * standing in is marked rather than being the only one you can see.
 *
 * Nothing here computes anything about a rule. Whether a rule is in force,
 * which of them could contradict each other, what fired and what came of it are
 * all stated by the server (§10.2, rule 2); this file decides where the words go.
 */
export function PlaybookScreen({ open, onClose }: { open: boolean; onClose: () => void }) {
  const playbook = useWorld((w) => w.playbook)
  const at = useWorld((w) => w.character?.at.key ?? '')
  const places = useWorld((w) => w.places)
  const busy = useWorld(sending)

  // Which set and which question this tab is looking at, and which part of the
  // sentence has its list open. All three are facts about this browser tab.
  const [chosen, setChosen] = useState<string | null>(null)
  const [asking, setAsking] = useState<string | null>(null)
  const [menu, setMenu] = useState<string | null>(null)

  if (!playbook || playbook.sets.length === 0 || playbook.questions.length === 0) return null

  // Where the player is standing, which is the set that matters to them today —
  // and only the default, because §3.15's failure is only legible if you can
  // read the answers you left somewhere else.
  const here = district(placeIn(places, at))
  const set = playbook.sets.find((s) => s.environment === (chosen ?? here)) ?? playbook.sets[0]
  const question = playbook.questions.find((q) => q.key === asking) ?? playbook.questions[0]
  const rule = set.rules.find((r) => r.question === question.key) ?? null

  const pair = (rule?.when.length ?? 0) > 1
  const pool = (part: Part) => (part === 'then' || part === 'else' ? question.actions : question.conditions)
  const refAt = (part: Part): PlaybookRef | null => {
    switch (part) {
      case 'c0':
        return rule?.when[0] ?? null
      case 'c1':
        return rule?.when[1] ?? null
      case 'then':
        return rule?.then ?? null
      case 'else':
        return rule?.otherwise ?? null
    }
  }

  /**
   * State the whole sentence, with one thing about it different.
   *
   * The whole of it every time, because that is the command: a per-part write
   * could leave the stored rule in a state nobody composed. `when` is sent with
   * its holes in it, since a hole is a real state of a rule rather than a value
   * this screen should be filling in.
   */
  function say(change: {
    when?: (PlaybookRef | null)[]
    join?: string
    then?: PlaybookRef | null
    otherwise?: PlaybookRef | null
  }) {
    // A second pick while the first is still in flight would compose from the
    // sentence as it stood *before* the answer landed, and undo it.
    if (!rule || busy) return
    setMenu(null)
    void composeRule({
      environment: set.environment,
      question: question.key,
      when: change.when ?? rule.when,
      join: change.join ?? rule.join,
      then: 'then' in change ? (change.then ?? null) : rule.then,
      otherwise: 'otherwise' in change ? (change.otherwise ?? null) : rule.otherwise,
    })
  }

  /**
   * The other branch's answer, when a pick would make both branches read the
   * same: what this side just held, or the first different action offered. A
   * rule that answers the same both ways is one whose `when` does no work, so
   * the sentence never lands in that state — picking the twin swaps the two.
   */
  function flipped(pick: PlaybookRef, held: PlaybookRef | null): PlaybookRef | null {
    if (held && held.term !== pick.term) return held
    const term = question.actions.find((t) => t.key !== pick.term)
    if (!term) return held
    return { term: term.key, value: term.values.length === 0 ? '' : term.values[0].key }
  }

  function put(part: Part, ref: PlaybookRef | null) {
    if (!rule) return
    if (part === 'then') {
      if (ref && rule.otherwise?.term === ref.term)
        return say({ then: ref, otherwise: flipped(ref, rule.then) })
      return say({ then: ref })
    }
    if (part === 'else') {
      if (ref && rule.then?.term === ref.term)
        return say({ then: flipped(ref, rule.otherwise), otherwise: ref })
      return say({ otherwise: ref })
    }
    const when: (PlaybookRef | null)[] = [rule.when[0] ?? null, ...(pair ? [rule.when[1] ?? null] : [])]
    when[part === 'c0' ? 0 : 1] = ref
    say({ when })
  }

  /**
   * Choose a term for one part of the sentence.
   *
   * A term that holds a number arrives with one already in it — the first of the
   * closed list the server sent, or the one that was there if this is the same
   * term again. Not an opinion about which threshold is right: the server refuses
   * a parameterised term with no number, and a menu that put the sentence into a
   * state the server would refuse would be offering a choice that does nothing.
   */
  function choose(part: Part, term: PlaybookTerm) {
    const was = refAt(part)
    const value =
      term.values.length === 0
        ? ''
        : was?.term === term.key && was.value
          ? was.value
          : term.values[0].key
    put(part, { term: term.key, value })
  }

  const answered = set.rules.filter((r) => r.inForce).length

  /** One part of the sentence, written out, with its number left live inside. */
  function written(part: Part) {
    const ref = refAt(part)
    const term = ref ? (pool(part).find((t) => t.key === ref.term) ?? null) : null
    if (!ref || !term) return null
    if (term.values.length === 0) return <>{term.says}</>

    const [before, after] = term.says.split(HOLE)
    const option = term.values.find((v) => v.key === ref.value) ?? null
    const spot = `num:${part}`

    return (
      <>
        {before}
        <span className="edit-anchor">
          <span
            className={`num${menu === spot ? ' open' : ''}`}
            role="button"
            tabIndex={0}
            onClick={(e) => {
              e.stopPropagation()
              setMenu(menu === spot ? null : spot)
            }}
          >
            {option?.says ?? '…'}
          </span>
          {menu === spot && (
            <Choices
              items={term.values.map((v) => ({
                label: v.says,
                on: v.key === ref.value,
                act: () => put(part, { term: ref.term, value: v.key }),
              }))}
            />
          )}
        </span>
        {after ?? ''}
      </>
    )
  }

  /**
   * One editable part, and the list it opens.
   *
   * `minor` is what this part can *become* rather than what it can say —
   * growing a second condition or dropping one. Never a way back to a blank:
   * a hole is where a part starts, not somewhere the menu offers to go, so
   * un-answering is only ever the whole sentence at once ("Take this answer
   * back").
   */
  function part(kind: 'cond' | 'act', which: Part, minor: Choice[] = []) {
    const ref = refAt(which)
    const showing = menu === which

    return (
      <span className="edit-anchor">
        <span
          className={`editable ${kind}${ref ? '' : ' empty'}${showing ? ' open' : ''}`}
          role="button"
          tabIndex={busy ? -1 : 0}
          onClick={(e) => {
            e.stopPropagation()
            setMenu(showing ? null : which)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') setMenu(showing ? null : which)
          }}
        >
          {ref ? written(which) : <span className="blank" />}
        </span>
        {showing && (
          <Choices
            items={pool(which).map((t) => ({
              label: offered(t, ref),
              on: ref?.term === t.key,
              act: () => choose(which, t),
            }))}
            minor={minor}
          />
        )}
      </span>
    )
  }

  const joint = (
    <span className="edit-anchor">
      <span
        className={`editable joint${menu === 'join' ? ' open' : ''}`}
        role="button"
        tabIndex={busy ? -1 : 0}
        onClick={(e) => {
          e.stopPropagation()
          setMenu(menu === 'join' ? null : 'join')
        }}
      >
        {rule?.join ?? 'and'}
      </span>
      {menu === 'join' && (
        <Choices
          items={[
            { label: 'and — both have to be true', on: rule?.join === 'and', act: () => say({ join: 'and' }) },
            { label: 'or — either one is enough', on: rule?.join === 'or', act: () => say({ join: 'or' }) },
          ]}
          minor={[
            {
              label: 'Drop the second condition',
              act: () => say({ when: [rule?.when[0] ?? null] }),
            },
          ]}
        />
      )}
    </span>
  )

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="Your playbook"
      sub="The world puts these questions to you while you are somewhere else, and your character
            answers them the way you said to. None of the answers is correct — each one works
            somewhere and ruins you somewhere else, which is why there is a separate set of them for
            every place you could be standing."
      // A click anywhere that is not a menu closes the one that is open. The
      // menus stop the click themselves, so this needs no outside listener.
      onBodyClick={() => setMenu(null)}
    >
      <div className="tabs">
        {playbook.sets.map((s) => (
          <a
            key={s.environment}
            className={s.environment === set.environment ? 'active' : ''}
            tabIndex={0}
            onClick={() => setChosen(s.environment)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') setChosen(s.environment)
            }}
          >
            {s.name} · {s.rules.filter((r) => r.inForce).length} answered
            {s.environment === here ? ' · you are here' : ''}
          </a>
        ))}
      </div>

      <div className="grid wide-left">
        <div className="stack">
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">Your answer, {set.name}</span>
              <span className="panel-note">{rule?.inForce ? 'in force here' : 'not covering this'}</span>
            </div>

            <div className="panel-body">
              <div className="ask">
                <span className="ask-n">{question.n}</span>
                <span>
                  <p className="ask-text">{question.ask}</p>
                  {!rule?.inForce && <p className="ask-note">{question.note}</p>}
                </span>
              </div>

              <div className="sentence">
                <span className="clause">
                  <span className="kw">when</span>
                  {part('cond', 'c0', [
                    pair
                      ? {
                          label: 'Drop this condition',
                          act: () => say({ when: [rule?.when[1] ?? null] }),
                        }
                      : {
                          label: '… and another condition',
                          act: () => say({ when: [rule?.when[0] ?? null, null] }),
                        },
                  ])}
                </span>

                {pair && (
                  <span className="clause">
                    {joint}
                    {part('cond', 'c1', [
                      {
                        label: 'Drop this condition',
                        act: () => say({ when: [rule?.when[0] ?? null] }),
                      },
                    ])}
                  </span>
                )}

                <span className="clause">
                  <span className="kw">then</span>
                  {part('act', 'then')}
                </span>

                <span className="clause">
                  <span className="kw">otherwise</span>
                  {part('act', 'else')}
                </span>
              </div>

              {!rule?.inForce && (
                <p className="note last">
                  While there is a blank in it, nothing is covering this question, and it comes
                  to you instead.
                </p>
              )}

              {/* Taking an answer back is its own thing, and not the same as
                  blanking every part of it: a sentence full of holes is a rule
                  you were in the middle of writing, and no rule at all is a
                  question you have never answered. */}
              <div className="btn-row">
                <button
                  className="btn ghost small"
                  disabled={busy}
                  onClick={() => {
                    setMenu(null)
                    void clearRule(set.environment, question.key)
                  }}
                >
                  Take this answer back
                </button>
              </div>
            </div>
          </div>

          <Meeting set={set} />
        </div>

        <div className="stack">
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">Standing questions</span>
              <span className="panel-note">
                {answered} of {set.rules.length} answered here
              </span>
            </div>

            <div className="panel-body">
              {playbook.questions.map((q) => {
                const held = set.rules.find((r) => r.question === q.key) ?? null
                return (
                  <div
                    key={q.key}
                    className={`question${q.key === question.key ? ' active' : ''}`}
                    role="button"
                    tabIndex={0}
                    onClick={() => setAsking(q.key)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') setAsking(q.key)
                    }}
                  >
                    <span className="question-n">{q.n}</span>
                    <span className="question-body">
                      <span className="question-ask">{q.ask}</span>
                      <span className="question-answer">
                        {held?.inForce ? (
                          <Answer question={q} rule={held} />
                        ) : (
                          <i>no answer — this one comes to you instead</i>
                        )}
                      </span>
                    </span>
                    <span className={`tag${held?.inForce ? '' : ' amber'}`}>
                      {held?.inForce ? 'In force' : 'Open'}
                    </span>
                  </div>
                )
              })}
            </div>
          </div>

          <Record open={open} questions={playbook.questions} />
        </div>
      </div>
    </Overlay>
  )
}

/**
 * A menu: the short list a part of the sentence opens, under the word.
 *
 * Never a modal and never a panel — the sentence stays readable behind it,
 * because the choice only makes sense in the sentence. It swallows its own
 * clicks so that picking something does not immediately reopen the part it
 * belongs to.
 */
export function Choices({
  items,
  minor = [],
}: {
  items: (Choice & { on?: boolean })[]
  minor?: Choice[]
}) {
  return (
    <span className="menu" onClick={(e) => e.stopPropagation()}>
      {items.map((i) => (
        <span
          key={i.label}
          className={`menu-item${i.on ? ' on' : ''}`}
          role="button"
          tabIndex={0}
          onClick={i.act}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') i.act()
          }}
        >
          {i.label}
        </span>
      ))}
      {minor.length > 0 && <span className="menu-rule" />}
      {minor.map((i) => (
        <span
          key={i.label}
          className="menu-item minor"
          role="button"
          tabIndex={0}
          onClick={i.act}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') i.act()
          }}
        >
          {i.label}
        </span>
      ))}
    </span>
  )
}

/**
 * Two of your answers that could disagree (§3.14).
 *
 * technical.md §6's argument for composing rules rather than writing them —
 * "these two both match and disagree is good UI, and impossible with prose" —
 * on the screen it was an argument for. Worked out by the server with no moment
 * in hand and sent beside the rules, so it moves the instant a word does.
 *
 * The sentence is the server's. This decides only that a pair which always
 * disagrees reads louder than a pair that might.
 */
function Meeting({ set }: { set: PlaybookSet }) {
  if (set.conflicts.length === 0) return null

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">Where these answers meet</span>
        <span className="panel-note">{set.name}</span>
      </div>

      <div className="panel-body">
        {set.conflicts.map((c) => (
          <div key={c.questions.join('+')} className={`notice${c.always ? '' : ' cool'}`}>
            {c.says}
          </div>
        ))}

        <p className="faint tiny">
          Worked out from the answers alone, before any of it happens. A moment can put more than
          one of these questions at once — somebody raising their voice while asking you to work
          late — and there is one answer per question, so that is the only way two of yours ever
          meet.
        </p>
      </div>
    </div>
  )
}

/** One rule, written out small enough to sit under the question it answers. */
function Answer({ question, rule }: { question: PlaybookQuestion; rule: PlaybookRule }) {
  return (
    <>
      <span className="kw">when</span>
      {rule.when.map((ref, i) => (
        <span key={i}>
          {i > 0 && <span className="kw"> {rule.join} </span>}
          <span className="token small cond">{phrase(termIn(question.conditions, ref), ref)}</span>
        </span>
      ))}
      <span className="kw">then</span>
      <span className="token small act">{phrase(termIn(question.actions, rule.then), rule.then)}</span>
      <span className="kw">otherwise</span>
      <span className="token small act">
        {phrase(termIn(question.actions, rule.otherwise), rule.otherwise)}
      </span>
    </>
  )
}

/**
 * What the rules did while you were elsewhere, and what a week of it adds up to
 * (§3.14's loop step 1, §3.6's pattern).
 *
 * Fetched when this screen opens rather than hydrated with the world: history
 * grows for as long as somebody plays, and a world model is bounded (§10.2). It
 * is the second pull-shaped surface in this client, after the leaderboards, and
 * it is thrown away with the component for the same reason a page of a board is.
 *
 * The counts are not a grade and are not presented as one. §3.15: one bad
 * outcome is noise and the same answer failing all week is a pattern — the
 * player is meant to be the one who notices, because §3.6's diagnosis is them
 * pointing at their own evidence rather than being told a conclusion.
 */
function Record({ open, questions }: { open: boolean; questions: PlaybookQuestion[] }) {
  const [page, setPage] = useState(1)
  const [log, setLog] = useState<PlaybookLog | null>(null)

  useEffect(() => {
    if (!open) return
    let live = true
    void loadPlaybookLog(page).then((got) => {
      if (live) setLog(got)
    })
    return () => {
      live = false
    }
  }, [open, page])

  const asked = (key: string) => questions.find((q) => q.key === key) ?? null

  return (
    <div className="panel">
      <div className="panel-head">
        <span className="panel-title">What your answers have done</span>
        <span className="panel-note">
          {log ? (log.total === 1 ? '1 moment' : `${log.total} moments`) : '—'}
        </span>
      </div>

      <div className="panel-body">
        {!log || log.total === 0 ? (
          <p className="note first">
            Nothing has come up yet. Situations arise while you are elsewhere — this is where you
            find out what your character did about them, and which of your answers decided it.
          </p>
        ) : (
          <>
            {log.trends.map((t) => (
              <div className="metric-row" key={t.question}>
                <div>
                  <div className="metric-name">
                    Answer {t.n} — {asked(t.question)?.about ?? t.question}
                  </div>
                  <div className="metric-sub">{counted(t)}</div>
                </div>
                <div className={`metric-value${t.poor > 0 ? ' warn' : ''}`}>{t.put}</div>
              </div>
            ))}

            {log.fires.map((f) => (
              <Moment key={f.id} fire={f} />
            ))}

            {log.pages > 1 && (
              <div className="btn-row">
                <button className="btn small" onClick={() => setPage(log.page - 1)} disabled={log.page <= 1}>
                  back
                </button>
                <span className="faint tiny">
                  page {log.page} of {log.pages}
                </span>
                <button
                  className="btn small"
                  onClick={() => setPage(log.page + 1)}
                  disabled={log.page >= log.pages}
                >
                  next
                </button>
              </div>
            )}

            <p className="faint tiny">
              Counted since {clockOf(log.since)} on {dayOf(log.since)}. One poor outcome is noise;
              the same answer failing all week is something you can do about.
            </p>
          </>
        )}
      </div>
    </div>
  )
}

/** One moment: what arose, which answers were weighed, and what followed. */
function Moment({ fire }: { fire: Fire }) {
  return (
    <div className="log-row">
      <span className="log-time">{clockOf(fire.at)}</span>
      <span>
        <span className="log-what">
          {fire.name || fire.situation} · {fire.environment}
        </span>
        {/*
          What happened, in the words it was written in — and what came of it.
          §3.14's step 2 wants four things on one line: the situation, which
          rule fired, the action, and the consequence. The two below are the
          first and the last, and both are sentences the server wrote: a client
          that summarised either would be composing an account of a moment it
          was not at.
        */}
        {fire.line && <span className="log-detail">{fire.line}</span>}
        {fire.considered.map((c) => (
          <span className="log-detail" key={c.question}>
            answer {c.n} {became(c.state)} — {c.says}
            {c.did ? `, so your character ${c.did}` : ''}
            {c.met.length > 0 ? ` (${c.met.map(truth).join(', ')})` : ''}
          </span>
        ))}
        {fire.note && <span className="log-detail">{fire.note}</span>}
        {fire.came && <span className="log-detail">{fire.came}</span>}
      </span>
      <span>
        <span className={`tag${fire.verdict === 'answered' ? '' : ' amber'}`}>{fire.verdict}</span>
        {fire.outcome !== 'pending' && (
          <span className={`tag ${fire.outcome === 'poor' ? 'warn-text' : 'ok-text'}`}>
            {fire.outcome}
          </span>
        )}
      </span>
    </div>
  )
}

/** One term, written out with its number in it — or a blank, for a hole. */
function phrase(term: PlaybookTerm | null, ref: PlaybookRef | null): string {
  if (!term || !ref) return '…'
  if (term.values.length === 0) return term.says
  const option = term.values.find((v) => v.key === ref.value)
  return term.says.replace(HOLE, option?.says ?? '…')
}

/**
 * A term as a menu offers it: written out in full, with the number it would
 * arrive holding — the one it already has if this is the term in the sentence,
 * its first authored value otherwise. A menu row is an offer, so it can never
 * be a blank: only a hole in the sentence reads as one.
 */
function offered(term: PlaybookTerm, ref: PlaybookRef | null): string {
  const held = ref?.term === term.key ? ref : null
  if (held) return phrase(term, held)
  if (term.values.length === 0) return term.says
  return term.says.replace(HOLE, term.values[0]?.says ?? '…')
}

/** A term out of one of a question's pools, by the key a rule names it with. */
function termIn(pool: PlaybookTerm[], ref: PlaybookRef | null): PlaybookTerm | null {
  if (!ref) return null
  return pool.find((t) => t.key === ref.term) ?? null
}

/** What became of one answer in one moment, in words rather than in a keyword. */
function became(state: string): string {
  switch (state) {
    case 'fired':
      return 'is what your character went by'
    case 'weighed':
      return 'also applied'
    case 'undecided':
      return 'could not be applied — the moment did not say enough'
    default:
      return 'had nothing to say'
  }
}

/** One condition, as the moment left it. Silence is its own answer here (§3.14). */
function truth(t: string): string {
  return t === 'met' ? 'met' : t === 'unmet' ? 'not met' : 'never mentioned'
}

/**
 * A week of one answer, counted.
 *
 * Every number the server sent and no verdict over them, because the verdict is
 * the player's — §3.2 labels nothing good or bad, and a sentence that concluded
 * something here would be doing §3.6's diagnosis for them.
 */
function counted(t: PlaybookTrend): string {
  const said = [
    `${t.put} came up`,
    `${t.fired} went by this answer`,
    t.undecided > 0 ? `${t.undecided} could not be applied` : '',
    t.unanswered > 0 ? `${t.unanswered} had no answer` : '',
    t.good > 0 ? `${t.good} went well` : '',
    t.poor > 0 ? `${t.poor} went badly` : '',
  ]
  return said.filter((s) => s !== '').join(' · ')
}
