import { useEffect, useState } from 'react'

import { KeyHint } from '../chrome/Keybar'
import { clearRule, composeRule } from '../world/actions'
import { useWorld } from '../world/react'
import {
  sending,
  type PlaybookQuestion,
  type PlaybookRef,
  type PlaybookSet,
  type PlaybookTerm,
  type World,
} from '../world/state'
import { Overlay } from './Overlay'

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

/**
 * The order the cards read down the page. A question the server sends that is
 * not named here comes after the ones that are, in the order the server sent.
 */
const ORDER = ['help', 'voice', 'money', 'overtime']

const inOrder = (questions: PlaybookQuestion[]): PlaybookQuestion[] => {
  const place = (q: PlaybookQuestion) => {
    const i = ORDER.indexOf(q.key)
    return i < 0 ? ORDER.length : i
  }
  return [...questions].sort((a, b) => place(a) - place(b))
}

/** Which part of the sentence is being edited. */
type Part = 'c0' | 'c1' | 'then' | 'else'

/**
 * Which menu is open, across the whole screen.
 *
 * A question and a part, because every question is on the page at once and one
 * open list at a time is the rule — two menus over four cards would be two
 * sentences being changed at the same time.
 */
type Spot = string
const spotOf = (question: string, part: string): Spot => `${question}:${part}`

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

  const open = set ? set.rules.filter((r) => !r.inForce).length : 0

  return (
    <KeyHint keys={['P']}>
      {open > 0 ? `your playbook — ${open} unanswered here` : 'your playbook'}
    </KeyHint>
  )
}

/**
 * The playbook: the rules you have set for your own judgement (§3.14).
 *
 * Three things about this screen are the design rather than the styling.
 *
 * Every question is a card, and all of them are on the world at once. There
 * are four of them and they fit, so there is nothing to pick from and nothing
 * moves under the cursor when a rule is changed.
 *
 * The editor is the sentence. There is no form and there is nothing to
 * press afterwards: every part of the rule opens a short list of what that part
 * could be, in place, under the word — so the rule stays readable while it is
 * being changed, and the change *is* the change. §3.14 gives the reason a save
 * button would be dishonest: the world is running behind this panel either way,
 * so a commit step would only mean something if the player could sit and think
 * in a paused world.
 *
 * It is a set of sets, and the tabs say so first. §3.15's whole thesis is
 * that the right answer depends on where you are, and a screen that opened on
 * one list of rules would teach the opposite before the player read a word. So
 * the environments come before the rules, every one of them exists from the
 * start — including the districts nobody has visited — and the one you are
 * standing in is marked rather than being the only one you can see.
 *
 * Nothing here computes anything about a rule. Whether a rule is in force is
 * stated by the server (§10.2, rule 2); this file decides where the words go.
 *
 * It is a writing surface and nothing else. Where the answers played out is in
 * the daily notifications, in the moment's own words — a ledger of tallies
 * under the sentence would turn deciding what your character does into reading
 * your own analytics.
 */
export function PlaybookScreen({
  open,
  onClose,
  at = null,
}: {
  open: boolean
  onClose: () => void
  /**
   * Which answer to open on, when the player was sent here from something that
   * one of their rules decided. A set and a question, which is the pair the
   * playbook is indexed by — the same two keys the server states everywhere
   * else, so nothing here has to work out which rule is meant.
   */
  at?: { environment: string; question: string } | null
}) {
  const playbook = useWorld((w) => w.playbook)
  const where = useWorld((w) => w.karma?.where ?? '')
  const busy = useWorld(sending)

  // Which set this tab is looking at, and which part of which sentence has its
  // list open. Both are facts about this browser tab.
  const [chosen, setChosen] = useState<string | null>(null)
  const [menu, setMenu] = useState<Spot | null>(null)

  // Opened on one answer. On the aim changing rather than on the screen
  // opening, for the reason the phone's is: an aim is one arrival.
  useEffect(() => {
    if (!at) return
    setChosen(at.environment)
    setMenu(null)
  }, [at])

  if (!playbook || playbook.sets.length === 0 || playbook.questions.length === 0) return null

  // The set actually in force, which is the one that matters to them today —
  // and only the default, because §3.15's failure is only legible if you can
  // read the answers you left somewhere else.
  //
  // `karma.where` and never the district the character is standing in. An
  // environment is a population and a population follows where you *live*, so
  // once relocation exists the two are routinely different — walk downtown for
  // an afternoon and a marker derived from your feet would put "you are here"
  // on a set that is not running.
  const set = playbook.sets.find((s) => s.environment === (chosen ?? where)) ?? playbook.sets[0]

  return (
    <Overlay
      open={open}
      onClose={onClose}
      title="Your playbook"
      sub="Define how your character will behave in different situations."
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
            {s.environment === where ? ' · you are here' : ''}
          </a>
        ))}
      </div>

      <div className="rules">
        {inOrder(playbook.questions).map((q) => (
          <Rule
            key={q.key}
            set={set}
            question={q}
            busy={busy}
            menu={menu}
            onMenu={setMenu}
          />
        ))}
      </div>
    </Overlay>
  )
}

/**
 * One question, and the sentence answering it where the player lives.
 *
 * Everything about composing a rule is in here, because a rule is what it
 * operates on: the parts, the menus under them, and the two guards that keep
 * the sentence in a state the server will take — a parameterised term always
 * arrives holding a number, and the two branches never say the same thing.
 */
function Rule({
  set,
  question,
  busy,
  menu,
  onMenu,
}: {
  set: PlaybookSet
  question: PlaybookQuestion
  busy: boolean
  menu: Spot | null
  onMenu: (spot: Spot | null) => void
}) {
  const rule = set.rules.find((r) => r.question === question.key) ?? null

  const pair = (rule?.when.length ?? 0) > 1
  const pool = (part: Part) =>
    part === 'then' || part === 'else' ? question.actions : question.conditions
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
    onMenu(null)
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
    const when: (PlaybookRef | null)[] = [
      rule.when[0] ?? null,
      ...(pair ? [rule.when[1] ?? null] : []),
    ]
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

  /** One part of the sentence, written out, with its number left live inside. */
  function written(part: Part) {
    const ref = refAt(part)
    const term = ref ? (pool(part).find((t) => t.key === ref.term) ?? null) : null
    if (!ref || !term) return null
    if (term.values.length === 0) return <>{term.says}</>

    const [before, after] = term.says.split(HOLE)
    const option = term.values.find((v) => v.key === ref.value) ?? null
    const spot = spotOf(question.key, `num:${part}`)

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
              onMenu(menu === spot ? null : spot)
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
   * `kind` is how the part reads rather than what it holds: the action the
   * character takes is the lit one, the fallback is an outline, and a condition
   * is a plain chip. `minor` is what the part can *become* — growing a second
   * condition or dropping one. Never a way back to a blank: a hole is where a
   * part starts, not somewhere the menu offers to go, so un-answering is only
   * ever the whole sentence at once ("Take this answer back").
   */
  function part(kind: 'cond' | 'act' | 'alt', which: Part, minor: Choice[] = []) {
    const ref = refAt(which)
    const spot = spotOf(question.key, which)
    const showing = menu === spot

    return (
      <span className="edit-anchor">
        <span
          className={`editable ${kind}${ref ? '' : ' empty'}${showing ? ' open' : ''}`}
          role="button"
          tabIndex={busy ? -1 : 0}
          onClick={(e) => {
            e.stopPropagation()
            onMenu(showing ? null : spot)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') onMenu(showing ? null : spot)
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

  const joinSpot = spotOf(question.key, 'join')
  const joint = (
    <span className="edit-anchor">
      <span
        className={`editable joint${menu === joinSpot ? ' open' : ''}`}
        role="button"
        tabIndex={busy ? -1 : 0}
        onClick={(e) => {
          e.stopPropagation()
          onMenu(menu === joinSpot ? null : joinSpot)
        }}
      >
        {rule?.join ?? 'and'}
      </span>
      {menu === joinSpot && (
        <Choices
          items={[
            {
              label: 'and — both have to be true',
              on: rule?.join === 'and',
              act: () => say({ join: 'and' }),
            },
            {
              label: 'or — either one is enough',
              on: rule?.join === 'or',
              act: () => say({ join: 'or' }),
            },
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
    <div className={`panel rule${rule?.inForce ? '' : ' open-rule'}`}>
      <div className="rule-head">
        <p className="rule-ask">{question.ask}</p>
        <span className={`rule-state${rule?.inForce ? '' : ' open'}`}>
          {rule?.inForce ? 'In force' : 'Open'}
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
          {part('alt', 'else')}
        </span>
      </div>

      {!rule?.inForce && <p className="note last">{question.note}</p>}

      {/* Taking an answer back is its own thing, and not the same as blanking
          every part of it: a sentence full of holes is a rule you were in the
          middle of writing, and no rule at all is a question you have never
          answered. */}
      {rule?.inForce && (
        <div className="btn-row">
          <button
            className="btn ghost small"
            disabled={busy}
            onClick={() => {
              onMenu(null)
              void clearRule(set.environment, question.key)
            }}
          >
            Take this answer back
          </button>
        </div>
      )}
    </div>
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

/** One term, written out with its number in it — or a blank, for a hole. Only a
 * menu row needs this now: the sentence itself is rendered live and editable. */
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
