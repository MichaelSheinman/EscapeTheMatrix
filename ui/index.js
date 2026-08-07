/* @etm/ui — components
   ===================================================================
   Each export returns an HTML string. That is a deliberate interim shape:
   it matches how `preview/` renders today, and every function here is a
   one-to-one translation target for a JSX component in `web/` later. The
   contract — the names, the option keys, the variant vocabulary — is what
   survives that port; the string building does not.

   Two rules for anyone adding to this file:

     1. Variants are semantic, never presentational. `variant: 'danger'`,
        never `variant: 'red'`. A variant name that describes the paint
        will be wrong the first time the paint changes.

     2. No content, no logic, no state. These render what they are handed.
        Copy lives in the caller (and eventually in `content/`, §5.2).

   Text arguments are treated as trusted authored HTML — entities and
   inline markup pass through unescaped, because the copy is repo-authored
   and often contains Token() spans. Never hand these player input.
   =================================================================== */


/* ─── internals ──────────────────────────────────────────────────── */

/** Join truthy class names. */
const cx = (...parts) => parts.filter(Boolean).join(' ');

/** Render a class attribute, or nothing when there are no classes. */
const klass = (...parts) => {
  const c = cx(...parts);
  return c ? ` class="${c}"` : '';
};

/** Render `data-*` and `style` attributes from an options object. */
const attrs = ({ panel, move, when, go, scene, pick, act, ask, slot, drawer, pose,
                 style, title } = {}) => cx(
  panel ? `data-panel="${panel}"` : '',
  move ? `data-move="${move}"` : '',
  when ? `data-when="${when}"` : '',
  go ? `data-go="${go}"` : '',
  scene ? `data-scene="${scene}"` : '',
  pick ? `data-pick="${pick}"` : '',
  act ? `data-act="${act}"` : '',
  ask ? `data-ask="${ask}"` : '',
  slot ? `data-slot="${slot}"` : '',
  drawer ? `data-drawer="${drawer}"` : '',
  pose ? `data-pose="${pose}"` : '',
  title ? `title="${title}"` : '',
  style ? `style="${style}"` : ''
).replace(/^(.)/, ' $1');

const join = (children) => (Array.isArray(children) ? children.join('') : (children || ''));


/* ══════════════════════════════════════════════════════════ PRIMITIVES */

/**
 * Button — an action.
 * @param variant 'default' | 'primary' | 'ghost' | 'spent'
 * @param size    'default' | 'small'
 *
 * Renders a <span>, not a <button>: the preview delegates every click from
 * the document root, and a real <button> inside the overlay would take
 * focus and steal the Escape key from the panel. `web/` should render a
 * real <button> — that difference is exactly why this is a component.
 */
export const Button = ({ label, variant, size, ...rest } = {}) =>
  `<span${klass('btn', variant, size === 'small' && 'small')}${attrs(rest)}>${label}</span>`;

/** A row of buttons. The first is the primary; there is only ever one. */
export const ButtonRow = (children, opts) =>
  `<div${klass('btn-row')}${attrs(opts)}>${join(children)}</div>`;

/**
 * Tag — a state, never an action.
 * @param variant 'default' | 'accent' | 'danger' | 'success' | 'dim'
 */
const TAG_VARIANT = { accent: 'amber', danger: 'warn-text', success: 'ok-text', dim: 'dim' };
export const Tag = ({ label, variant } = {}) =>
  `<span${klass('tag', TAG_VARIANT[variant])}>${label}</span>`;

/**
 * Token — a term from the rule vocabulary.
 * @param kind 'cond' | 'act' | 'locked'
 * @param size 'default' | 'small'
 * @param act  makes it a choice the player can take: it gains a hover and a
 *             pointer, which a token sitting inside a composed rule must not
 * @param on   currently chosen — only meaningful alongside `act`
 */
export const Token = ({ label, kind = 'cond', size, on, ...rest } = {}) =>
  `<span${klass('token', kind, size === 'small' && 'small',
                rest.act && 'pickable', on && 'on')}${attrs(rest)}>${label}</span>`;

/** A wrapped set of tokens. */
export const Tokens = (children) => `<div${klass('tokens')}>${join(children)}</div>`;

/** Kw — rule grammar (WHEN / AND / THEN). Must never look clickable. */
export const Kw = (word) => `<span${klass('kw')}>${word}</span>`;

/**
 * Ref — a condition position in an expression, before anything fills it.
 * The A / B / C the player is arranging. It is the same mark in the shape
 * picker and in the composed line, which is what makes the picker legible:
 * you choose the sentence first and say what A is afterwards.
 */
export const Ref = (letter) => `<span${klass('ref')}>${letter}</span>`;

/**
 * Group — a parenthesised sub-expression. The brackets are drawn by the
 * stylesheet rather than typed, so grouping survives a change of notation.
 */
export const Group = (children) => `<span${klass('group')}>${join(children)}</span>`;


/* ═══════════════════════════════════════════════════════════ DISPLAY */

/**
 * Panel — the standard surface.
 * @param title   uppercase label, optional; omit for a headless panel
 * @param note    right-hand metadata text
 * @param tag     right-hand Tag options, as an alternative to `note`
 * @param bodyClass extra classes on the body (e.g. 'readout')
 */
export const Panel = ({ title, note, tag, body, bodyClass, ...rest } = {}) => {
  const right = note ? `<span${klass('panel-note')}>${note}</span>`
    : tag ? Tag(tag)
    : '';
  const head = title
    ? `<div${klass('panel-head')}><span${klass('panel-title')}>${title}</span>${right}</div>`
    : '';
  return `<div${klass('panel')}${attrs(rest)}>${head}` +
    `<div${klass('panel-body', bodyClass)}>${join(body)}</div></div>`;
};

/**
 * Stack — vertical rhythm between siblings.
 * Replaces the inline `style="margin-bottom:14px"` that was repeated on
 * every non-final panel in a column.
 */
export const Stack = (children, opts) =>
  `<div${klass('stack')}${attrs(opts)}>${join(children)}</div>`;

/**
 * Grid — page-level column layout.
 * @param variant 'two' (equal) | 'wide-left' (1.5fr 1fr)
 */
export const Grid = ({ variant, children, ...rest } = {}) =>
  `<div${klass('grid', variant)}${attrs(rest)}>${join(children)}</div>`;

/** Readout — a Stat beside supporting content. */
export const Readout = (children) => `<div${klass('readout')}>${join(children)}</div>`;

/** Stat — the one big number. At most one per panel. */
export const Stat = ({ value, unit, accent } = {}) =>
  `<div><div${klass('stat-big', accent && 'accent')}>${value}</div>` +
  `<div${klass('stat-unit')}>${unit}</div></div>`;

/**
 * Metric — name + sub / value / bar.
 * @param tone    'default' | 'warn' | 'ok' | 'accent'  (colours the value)
 * @param barTone 'default' | 'warn' | 'ok'             (colours the bar)
 * @param plain   drop the divider — for metrics stacked inside a Readout
 */
export const Metric = ({ name, sub, value, tone, pct, barTone, barOpacity, plain } = {}) =>
  `<div${klass('metric-row', plain && 'plain')}>` +
    `<div><div${klass('metric-name')}>${name}</div>` +
    (sub ? `<div${klass('metric-sub')}>${sub}</div>` : '') + `</div>` +
    `<div${klass('metric-value', tone)}>${value}</div>` +
    Bar({ pct, tone: barTone, opacity: barOpacity }) +
  `</div>`;

/** Bar — a proportion. @param tone 'default' | 'warn' | 'ok' */
export const Bar = ({ pct, tone, opacity } = {}) =>
  `<div${klass('bar')}><span${klass(tone)} style="width:${pct}%` +
  (opacity ? `;opacity:${opacity}` : '') + `"></span></div>`;

/**
 * Sparkline — a trend against a target.
 * Colours come from --ink-* rather than literals, so the chart is inside
 * the art bible instead of beside it.
 * @param points array of [x, y] in the 280×62 viewBox
 * @param mark   [x, y] to dot — the reading being pointed at
 */
export const Sparkline = ({ points = [], target, targetLabel, mark, from, to } = {}) =>
  `<div${klass('spark')}><svg viewBox="0 0 280 62" preserveAspectRatio="none">` +
    (target !== undefined
      ? `<line x1="0" y1="${target}" x2="280" y2="${target}" stroke="var(--ink-axis)" stroke-dasharray="3 4"/>`
      : '') +
    `<polyline fill="none" stroke="var(--ink-plot)" stroke-width="1.6" stroke-linejoin="round" ` +
    `points="${points.map((p) => p.join(',')).join(' ')}"/>` +
    (mark ? `<circle cx="${mark[0]}" cy="${mark[1]}" r="2.6" fill="var(--ink-plot-mark)"/>` : '') +
    (targetLabel
      ? `<text x="0" y="10" fill="var(--ink-axis-text)" font-size="8" font-family="monospace">${targetLabel}</text>`
      : '') +
  `</svg><div${klass('spark-axis')}><span>${from || ''}</span><span>${to || ''}</span></div></div>`;

/**
 * LockedSlot — something the simulation is computing that the player has
 * no instrument to read. The hatch means occupied, not empty.
 */
export const LockedSlot = ({ name, why, dim } = {}) =>
  `<div${klass('locked-slot', dim && 'dim')}>` +
  `<span${klass('locked-name')}>${name}</span>` +
  `<span${klass('locked-why')}>${why}</span></div>`;

/** A group of LockedSlots. */
export const Slots = (children, opts) =>
  `<div${klass('slots')}${attrs(opts)}>${join(children)}</div>`;

/** Notice — an inline banner. @param tone 'warn' (default) | 'cool' */
export const Notice = ({ text, tone } = {}) =>
  `<div${klass('notice', tone === 'cool' && 'cool')}>${text}</div>`;

/** Tabs — @param items [{ label, active, panel, faint }] */
export const Tabs = (items = []) =>
  `<div${klass('tabs')}>` + items.map((t) =>
    `<a${klass(t.active && 'active', t.faint && 'faint')}${attrs(t)} href="#">${t.label}</a>`
  ).join('') + `</div>`;

/** A paragraph of supporting copy. Saves repeating `class="faint tiny"`. */
export const Note = (text, { first, last, style } = {}) =>
  `<p${klass('faint', 'tiny', first && 'mt0', last && 'mb0')}` +
  (style ? ` style="${style}"` : '') + `>${text}</p>`;

/**
 * Prose — a paragraph at body weight.
 * @param size  'default' | 'tiny'
 * @param sub   a muted second line beneath the text
 * @param first drop the top margin (first child of a panel body)
 */
export const Prose = (text, { size, sub, first, last } = {}) =>
  `<p${klass(size === 'tiny' && 'tiny', first && 'mt0', last && 'mb0')}>${text}` +
  (sub ? `<br><span${klass('faint', 'tiny')}>${sub}</span>` : '') + `</p>`;

/** Divider — a hairline rule between sections of a panel body. */
export const Divider = () => `<hr${klass('soft')}>`;


/* ══════════════════════════════════════════════════════════════ ROWS */

/**
 * LogRow — time / title + detail / trailing.
 * The workhorse: inbox entries, resolved conflicts, shift reports.
 * @param trailing a Tag() or Button() string
 * @param dim      the title greys out — declined, walked past
 */
export const LogRow = ({ time, title, detail, trailing, dim } = {}) =>
  `<div${klass('log-row')}><span${klass('log-time')}>${time}</span>` +
  `<span><span${klass('log-what', dim && 'faint')}>${title}</span>` +
  (detail ? `<span${klass('log-detail')}>${detail}</span>` : '') + `</span>` +
  (trailing || '') + `</div>`;

/** BoardRow — a standing on a leaderboard. @param variant 'you' | 'gap' */
export const BoardRow = ({ rank, name, where, value, tone, variant } = {}) =>
  `<div${klass('board', variant)}><span${klass('rank')}>${rank || ''}</span>` +
  `<span>${name ? `<b>${name}</b>` : ''}${where ? `<i>${where}</i>` : ''}</span>` +
  `<span${klass('metric-value', tone)}>${value || ''}</span></div>`;

/** LadderRow — a rung. @param variant 'current' | 'future' */
export const LadderRow = ({ tier, title, detail, trailing, variant } = {}) =>
  `<div${klass('ladder-row', variant)}><span${klass('ladder-tier')}>${tier}</span>` +
  `<span><b>${title}</b><i>${detail}</i></span>${trailing || ''}</div>`;

/** DecayRow — a labelled magnitude, for showing returns falling over time. */
export const DecayRow = ({ label, pct, amount } = {}) =>
  `<div${klass('decay-row')}><span${klass('label')}>${label}</span>` +
  `<span${klass('decay-track')}><span style="width:${pct}%"></span></span>` +
  `<span${klass('amount')}>${amount}</span></div>`;

/**
 * Posting — a job on the board.
 * @param reqs [{ label, state }] where state is 'met' | 'unmet' | 'hidden'
 */
export const Posting = ({ role, where, pay, payNote, hours, reqs = [], foot, action } = {}) =>
  `<div${klass('posting')}><div${klass('posting-head')}>` +
    `<div><div${klass('posting-role')}>${role}</div>` +
    `<div${klass('posting-where')}>${where}</div></div>` +
    `<div${klass('right')}><div${klass('posting-pay')}>${pay}` +
    (payNote ? ` <span${klass('faint', 'tiny')}>${payNote}</span>` : '') + `</div>` +
    `<div${klass('posting-where')}>${hours}</div></div></div>` +
    `<div${klass('req')}>` +
      reqs.map((r) => `<span${klass(r.state)}>${r.label}</span>`).join('') +
    `</div>` +
    `<div${klass('posting-foot')}><span${klass('faint', 'tiny')}>${foot}</span>${action || ''}</div>` +
  `</div>`;


/* ═════════════════════════════════════════════════════════ COMPOSITES */

/**
 * Rule — the playbook atom.
 *
 * Two ways to state the left-hand side, and they are the same object at two
 * ages. `when` is the flat form — a list of conditions joined with AND, which
 * is every rule the player wrote before the builder existed. `about` + `expr`
 * is the composed form: the situation the rule answers, and an arbitrary
 * expression over it built from Token / Kw / Group / Ref by the caller.
 *
 * @param when      array of condition labels; joined with AND
 * @param about     the situation this rule answers, as one condition label
 * @param expr      pre-composed expression HTML, shown after IF
 * @param then      the action label
 * @param otherwise the action taken when the expression does not hold
 * @param meta      array of strings, or { text, tone } for the failing ones
 * @param variant   'draft' — being composed, not yet in force
 */
export const Rule = ({ when = [], about, expr, then, otherwise,
                      meta = [], conflict, variant, ...rest } = {}) => {
  const left = about
    ? Token({ label: about, kind: 'cond' }) + (expr ? Kw('if') + expr : '')
    : when.map((c, i) =>
        (i ? Kw('and') : '') + Token({ label: c, kind: 'cond' })
      ).join('');
  const metaLine = meta.length
    ? `<div${klass('rule-meta')}>` + meta.map((m) =>
        typeof m === 'string' ? `<span>${m}</span>`
          : `<span${klass(m.tone === 'danger' && 'warn-text')}>${m.text}</span>`
      ).join('') + `</div>`
    : '';
  return `<div${klass('rule', conflict && 'conflict', variant)}${attrs(rest)}>` +
    `<div${klass('rule-line')}>${Kw('when')}${left}${Kw('then')}` +
    Token({ label: then, kind: 'act' }) +
    (otherwise ? Kw('otherwise') + Token({ label: otherwise, kind: 'act' }) : '') +
    `</div>${metaLine}</div>`;
};

/** SceneHead — challenge framing: what it is, how far in, what it costs. */
export const SceneHead = ({ tag, stages = [], note } = {}) =>
  `<div${klass('scene-head')}>` + (tag ? Tag(tag) : '') +
  `<span${klass('stage-dots')}>` +
  stages.map((s) => `<i${klass(s === 'done' && 'done', s === 'now' && 'now')}></i>`).join('') +
  `</span><span${klass('faint', 'tiny')}>${note}</span></div>`;

/** Dialogue — portrait beside speech. */
export const Dialogue = ({ who, name, role, children } = {}) =>
  `<div${klass('dialogue')}><div${klass('portrait')}>${Portrait(who)}` +
  `<div${klass('portrait-name')}>${name}<div${klass('portrait-role')}>${role}</div></div></div>` +
  `<div>${join(children)}</div></div>`;

/**
 * Speech — a line of dialogue.
 * @param size    'default' | 'small'
 * @param variant 'aside' — the other party in the room, quieter than the speaker
 */
export const Speech = (text, { size, variant, first } = {}) =>
  `<p${klass('speech', size === 'small' && 'speech-sm', variant, first && 'mt0')}>${text}</p>`;

/**
 * Option — an authored, committable reading. Never marked correct.
 * Also what you can say to somebody: a question out of a closed list is the
 * same object as a reading out of a closed list, and neither is typed.
 */
export const Option = ({ lead = 'Commit to this', text, chosen, ...rest } = {}) =>
  `<div${klass('option', chosen && 'chosen')}${attrs(rest)}>` +
  `<div${klass('option-lead')}>${lead}</div>` +
  `<div${klass('option-text')}>${text}</div></div>`;

/**
 * Reply — what somebody says back when you ask them one of their questions.
 * Sits directly under the question rather than replacing it, so a short
 * closed list reads as a conversation instead of a menu that changed page.
 * The attribution is part of it: an answer in this world belongs to a named
 * person with a handle and a number, never to the game.
 */
export const Reply = ({ name, text } = {}) =>
  `<div${klass('reply')}>` +
    Speech(text, { first: true }) +
    `<p${klass('reply-who')}>${name}</p>` +
  `</div>`;

/** Chip — a piece of the player's own evidence, selectable in a diagnosis. */
export const Chip = ({ src, val, picked } = {}) =>
  `<div${klass('chip', picked && 'picked')}>` +
  `<span${klass('chip-src')}>${src}</span>` +
  `<span${klass('chip-val')}>${val}</span></div>`;

/** A wrapped set of Chips. */
export const Chips = (children) => `<div${klass('chips')}>${join(children)}</div>`;

/** Commit — the confirmation step below a set of Chips. */
export const Commit = (children) => `<div${klass('commit')}>${join(children)}</div>`;

/* ═══════════════════════════════════════════════════════════ BUILDER

   The surface the playbook is written on, and it is one sentence:

     WHEN [something] THEN [do this], otherwise [do that]

   There is no form around it and no separate preview of it. The player
   clicks a part of the sentence and a short list of what that part could be
   opens under it. Nothing accepts free text — that is what keeps a rule
   something the simulation can run and the player can diagnose afterwards —
   so nothing here needs a field, a label or a save row.

   The one structural idea is that editable things nest: a condition is
   clickable, and the number inside that condition is clickable on its own.
   Both are the same object at different grains, which is why they are one
   component (Editable) plus a smaller one (Num) rather than a special case. */

/**
 * Ask — the standing question an answer responds to.
 * The questions are authored and there are few of them, so each one carries
 * its number: the player learns the playbook as "my answer to 2", not as a
 * list of rules that happen to exist.
 */
export const Ask = ({ n, text, note } = {}) =>
  `<div${klass('ask')}><span${klass('ask-n')}>${n}</span><div>` +
    `<p${klass('ask-text')}>${text}</p>` +
    (note ? `<p${klass('ask-note')}>${note}</p>` : '') +
  `</div></div>`;

/** Sentence — the rule, written out, with its parts live. */
export const Sentence = (children, opts) =>
  `<div${klass('sentence')}${attrs(opts)}>${join(children)}</div>`;

/**
 * Clause — a keyword and the part it introduces, wrapping as one thing.
 * A sentence that breaks between THEN and what follows it has stopped being
 * a sentence, so the two never become separate items in the wrap.
 */
export const Clause = (children) =>
  `<span${klass('clause')}>${join(children)}</span>`;

/**
 * Editable — one part of the sentence the player can change.
 * @param kind  'cond' | 'act' | 'joint' — what it is in the grammar
 * @param body  composed content, which may itself contain a Num
 * @param empty nothing chosen yet: it reads as a blank in the sentence
 * @param menu  the Menu it currently has open, anchored beneath it
 */
export const Editable = ({ kind = 'cond', body, empty, menu, ...rest } = {}) =>
  `<span${klass('edit-anchor')}>` +
    `<span${klass('editable', kind, empty && 'empty', menu && 'open')}${attrs(rest)}>` +
      (empty ? `<span${klass('blank')}></span>` : body) +
    `</span>` +
    (menu || '') +
  `</span>`;

/**
 * Num — the number inside an Editable, editable on its own.
 * The nesting is the point: clicking the condition changes *which* condition,
 * clicking the number changes only the threshold. The hover states have to
 * make that difference obvious before the click, which is why the outer
 * highlight stands down while the pointer is on the inner one.
 */
export const Num = ({ label, menu, ...rest } = {}) =>
  `<span${klass('edit-anchor')}>` +
    `<span${klass('num', menu && 'open')}${attrs(rest)}>${label}</span>` +
    (menu || '') +
  `</span>`;

/**
 * Menu — the short list an Editable opens, in place, under the word it
 * belongs to. Never a modal and never a panel: the sentence stays readable
 * behind it, because the choice only makes sense in the sentence.
 *
 * It holds the choices and nothing else. No explanatory line, and no row
 * standing in for a choice that is not available yet — both make the menu
 * taller than the sentence it is covering, which is the one thing a menu
 * anchored to a word cannot afford to be.
 *
 * @param items [{ label, act, on }]
 * @param extra [{ label, act }] — what this part can become, below a divider
 */
export const Menu = ({ items = [], extra = [] } = {}) =>
  `<span${klass('menu')}>` +
    items.map((i) =>
      `<span${klass('menu-item', i.on && 'on')}${attrs(i)}>${i.label}</span>`
    ).join('') +
    (extra.length
      ? `<span${klass('menu-rule')}></span>` + extra.map((i) =>
          `<span${klass('menu-item', 'minor')}${attrs(i)}>${i.label}</span>`
        ).join('')
      : '') +
  `</span>`;

/**
 * Question — a standing question and whatever currently answers it.
 * @param answer the composed rule line, or the reason there isn't one
 * @param state  Tag options for how the answer stands
 * @param active the one being edited
 */
export const Question = ({ n, ask, answer, state, active, ...rest } = {}) =>
  `<div${klass('question', active && 'active')}${attrs(rest)}>` +
    `<span${klass('question-n')}>${n}</span>` +
    `<span${klass('question-body')}>` +
      `<span${klass('question-ask')}>${ask}</span>` +
      `<span${klass('question-answer')}>${answer}</span>` +
    `</span>` +
    (state ? Tag(state) : '') +
  `</div>`;


/* ═════════════════════════════════════════════════════════════ SHELL

   There is no navigation rail and no top bar. The canvas is the whole
   screen and every one of these floats over it, because a destination you
   can walk to must not also exist as a menu item — the moment it does,
   walking there is the slow way to do it. What survives as chrome is only
   what the world cannot say for itself: your standing, what is reachable
   from where you stand, and which key does it. */

/**
 * StatPod — one persistent readout, floated over the world.
 * Reads at a glance from across the screen: glyph, number, quiet label.
 * @param tone  'accent' | 'warn' | undefined
 * @param note  a small trailing qualifier ("/ 3", "+4 wk")
 */
export const StatPod = ({ glyph, value, note, label, tone, ...rest } = {}) =>
  `<div${klass('pod', tone)}${attrs(rest)}>` +
    `<span${klass('pod-glyph')}>${glyph}</span>` +
    `<span${klass('pod-text')}>` +
      `<span${klass('pod-value')}>${value}` +
        (note ? `<span${klass('pod-note')}>${note}</span>` : '') +
      `</span>` +
      `<span${klass('pod-label')}>${label}</span>` +
    `</span>` +
  `</div>`;

/** The stack of pods down one edge of the screen. */
export const StatPods = (children) => `<div${klass('pods')}>${join(children)}</div>`;

/**
 * PlaceChip — where you are and what time it is.
 * The only orientation the shell gives you; everything else is diegetic.
 */
export const PlaceChip = ({ name, where, clock } = {}) =>
  `<span${klass('place-name')}>${name}</span>` +
  `<span${klass('place-where')}>${where} &middot; ${clock}</span>`;

/** EnvChip — which rule set is in force where you are standing. */
export const EnvChip = ({ label, name, rules, ruleTone } = {}) =>
  `<span${klass('env-label')}>${label}</span>` +
  `<span${klass('env-name')}>${name}</span>` +
  `<span${klass('tag', ruleTone)}>${rules}</span>`;

/**
 * PhoneButton — the one pushed surface, parked in the corner.
 * Everything else finds you because you walked into it; this is the
 * exception, so it is the only thing on screen that carries a count.
 *
 * The handset is drawn rather than typed: the glyph fonts ship for a phone
 * is a 1960s desk set, and the thing in your character's pocket is not that.
 */
export const PhoneButton = ({ count, key } = {}) =>
  `<svg${klass('phone-glyph')} viewBox="0 0 24 24" aria-hidden="true">` +
    `<rect x="6.5" y="2.5" width="11" height="19" rx="2.6"` +
    ` fill="none" stroke="currentColor" stroke-width="1.5"/>` +
    `<rect x="8.6" y="5.4" width="6.8" height="11.2" rx="0.9"` +
    ` fill="currentColor" opacity="0.16"/>` +
    `<line x1="10.4" y1="4" x2="13.6" y2="4"` +
    ` stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>` +
    `<circle cx="12" cy="19.1" r="0.95" fill="currentColor"/>` +
  `</svg>` +
  (count ? `<span${klass('phone-badge')}>${count}</span>` : '') +
  (key ? `<span${klass('corner-key')}>${key}</span>` : '');

/**
 * HelpButton — every key on the keyboard, one click away.
 * Sits beside the phone because both are things you reach for rather than
 * things the world hands you.
 */
export const HelpButton = ({ key } = {}) =>
  `<span${klass('help-glyph')}>?</span>` +
  (key ? `<span${klass('corner-key')}>${key}</span>` : '');

/**
 * Toast — something arrived while you were walking.
 * Dismissible and ignorable by construction: it never blocks the world,
 * and letting it expire drops it into the phone rather than losing it.
 */
export const Toast = ({ kicker, title, body, action, panel } = {}) =>
  `<div${klass('toast-head')}>` +
    `<span${klass('toast-kicker')}>${kicker}</span>` +
    `<span${klass('toast-x')} data-toast-close>&#10005;</span>` +
  `</div>` +
  `<p${klass('toast-title')}>${title}</p>` +
  (body ? `<p${klass('toast-body')}>${body}</p>` : '') +
  (action ? Button({ label: action, size: 'small', panel }) : '');

/**
 * ActionPrompt — what the thing you are standing next to is called, and
 * the key that acts on it. Pinned over the world by the renderer.
 */
export const ActionPrompt = ({ label, hint, key } = {}) =>
  `<span${klass('prompt-key')}>${key}</span>` +
  `<span${klass('prompt-text')}>` +
    `<span${klass('prompt-label')}>${label}</span>` +
    (hint ? `<span${klass('prompt-hint')}>${hint}</span>` : '') +
  `</span>`;

/** Keycap — one key, drawn as a key. */
export const Keycap = (k) => `<kbd${klass('keycap')}>${k}</kbd>`;

/**
 * KeyBar — the always-on reminder line.
 * @param items [{ keys: ['A'], label: 'apartment' }]
 */
export const KeyBar = (items = []) =>
  items.map(({ keys, label }) =>
    `<span${klass('keybar-item')}>${keys.map(Keycap).join('')}` +
    `<span${klass('keybar-label')}>${label}</span></span>`
  ).join('');

/**
 * KeyCard — the full legend, toggled with ?.
 * @param groups [{ title, items: [{ keys, label }] }]
 */
export const KeyCard = (groups = []) =>
  groups.map(({ title, items }) =>
    `<div${klass('keycard-group')}>` +
      `<p${klass('keycard-title')}>${title}</p>` +
      items.map(({ keys, label }) =>
        `<div${klass('keycard-row')}>` +
          `<span${klass('keycard-keys')}>${keys.map(Keycap).join('')}</span>` +
          `<span${klass('keycard-label')}>${label}</span>` +
        `</div>`).join('') +
    `</div>`).join('');

/**
 * Marker — the label-and-dot pinned over a point in the world, and the
 * popup that says what the place is.
 *
 * `tip` is not optional. A labelled point the player cannot interrogate is
 * worse than an unlabelled one: it reads as a destination and then refuses
 * to say what it is for. Every location in the world carries a sentence,
 * including the ones you can only look at.
 *
 * @param kind 'portal' (a door) | 'use' (a thing) | 'sign' (a name only)
 * @param tip  one sentence, in the world's voice
 * @param key  the key that acts on it, where one does
 */
const MARKER_KIND = {
  portal: { kicker: 'Somewhere you can go in' },
  use: { kicker: 'Something you can use' },
  sign: { kicker: 'Just a name on a building', foot: 'Nothing to do here.' },
  // The city is allowed to be larger than the client. A place the design
  // calls for and the build has not reached is shown, named and explained
  // rather than left off the map.
  soon: { kicker: 'Somewhere you will be able to go', foot: 'Not built yet.' },
};

export const Marker = ({ label, tip, kind = 'use', key } = {}) => {
  const k = MARKER_KIND[kind] || MARKER_KIND.use;
  const foot = k.foot
    ? `<span${klass('marker-tip-foot')}>${k.foot}</span>`
    : `<span${klass('marker-tip-foot')}>${Keycap(key || 'O')}` +
      `<span>${kind === 'portal' ? 'or walk into the doorway' : 'when you are next to it'}</span></span>`;
  return `<span${klass('marker-tip')}>` +
      `<span${klass('marker-tip-kicker')}>${k.kicker}</span>` +
      `<p${klass('marker-tip-name')}>${label}</p>` +
      `<p${klass('marker-tip-body')}>${tip}</p>` +
      foot +
    `</span>` +
    `<span${klass('marker-label')}>${label}</span><span${klass('marker-dot')}></span>`;
};

/**
 * NamePlate — which of these people is you.
 * Rides over the character's head. Deliberately the only nameplate in the
 * world: everyone else you learn to recognise by where they stand and what
 * they are wearing, which is what makes dressing yourself matter.
 */
export const NamePlate = ({ name } = {}) =>
  `<span${klass('nameplate-name')}>${name}</span>` +
  `<span${klass('nameplate-tick')}></span>`;

/**
 * PersonTip — who that is, on hover.
 * The one thing the world will tell you about somebody without being asked
 * twice: their name and where they stand on the same board you are on. It is
 * pull, not push — it exists only while the pointer is on them, which is why
 * it can carry a number that a permanent label over every head could not.
 *
 * `karma` is pre-formatted by the caller; `note` is one clause on who they
 * are. `hint` is the one thing you can do about them — it belongs on the tip
 * rather than on a permanent label, for the same reason the rest of it does.
 */
export const PersonTip = ({ name, karma, note, hint } = {}) =>
  `<span${klass('person-tip-body')}>` +
    `<span${klass('person-tip-name')}>${name}</span>` +
    (note ? `<span${klass('person-tip-note')}>${note}</span>` : '') +
    (hint ? `<span${klass('person-tip-hint')}>${hint}</span>` : '') +
    `<span${klass('person-tip-karma')}>` +
      `<span${klass('person-tip-glyph')}>&#9672;</span>${karma}` +
      `<span${klass('person-tip-unit')}>Karma</span>` +
    `</span>` +
  `</span>` +
  `<span${klass('person-tip-tick')}></span>`;

/**
 * Bark — a line somebody says out loud, over their head, in the world.
 * Not dialogue: nothing is waiting on you and there is nothing to answer.
 * It is how a street sounds like it has people in it rather than pedestrians.
 */
export const Bark = (text) => `<span${klass('bark-body')}>${text}</span>`;

/**
 * Swatch — one thing you could be wearing.
 * @param color the paint, or omitted for a "none" option
 * @param on    currently worn
 * @param pick  "slot:value", read back by the client
 */
export const Swatch = ({ label, color, on, ...rest } = {}) =>
  `<span${klass('swatch', on && 'on', !color && 'none')}${attrs(rest)}>` +
    `<span${klass('swatch-chip')}${color ? ` style="background:${color}"` : ''}></span>` +
    `<span${klass('swatch-label')}>${label}</span>` +
  `</span>`;

/** A row of swatches — one slot's worth of choices. */
export const Swatches = (children) => `<div${klass('swatches')}>${join(children)}</div>`;

/**
 * Wardrobe — the closet's page shape: you above, the drawers below.
 * A column that fills its panel, so the rack sits on the bottom edge of the
 * screen no matter how little is in it. Nothing here is a step in a flow:
 * there is no preview, no confirm and no way out other than walking away,
 * because the change has already happened by the time you have seen it.
 */
export const Wardrobe = ({ mirror, rack } = {}) =>
  `<div${klass('wardrobe')}>` +
    `<div${klass('wardrobe-mirror')}>${join(mirror)}</div>` +
    `<div${klass('wardrobe-rack')}>${join(rack)}</div>` +
  `</div>`;

/**
 * Mirror — you, live, while you dress.
 * The stage is a canvas the consumer draws into with the same routine that
 * draws the figure in the street, so what you are looking at is not a
 * rendering of the choice — it is the choice, already made and already true
 * of the person walking around behind the scrim.
 *
 * @param name     what the boards call you, plated over the figure's head
 * @param controls authored HTML under the stage (how the figure is posed)
 * @param note     one quiet line under the stage
 */
export const Mirror = ({ name, controls, note } = {}) =>
  `<div${klass('mirror')}>` +
    `<div${klass('mirror-stage')}>` +
      `<canvas${klass('mirror-figure')}></canvas>` +
      `<span${klass('mirror-name')}>${NamePlate({ name })}</span>` +
    `</div>` +
    (controls ? `<div${klass('mirror-controls')}>${join(controls)}</div>` : '') +
    (note ? `<p${klass('mirror-note')}>${note}</p>` : '') +
  `</div>`;

/**
 * Rack — the drawers under the mirror, and the one that is open.
 * Every drawer is rendered and all but one are put away, so changing which
 * slot you are picking from is a class toggle rather than a re-render: the
 * figure above never blinks.
 */
export const Rack = ({ tabs, drawers, note } = {}) =>
  `<div${klass('rack')}>` +
    `<div${klass('rack-tabs')}>${join(tabs)}</div>` +
    `<div${klass('rack-drawers')}>${join(drawers)}</div>` +
    (note ? `<p${klass('rack-note', 'faint', 'tiny', 'mb0')}>${note}</p>` : '') +
  `</div>`;

/**
 * RackTab — one drawer's handle: which slot it is, and what is in it now.
 * It carries the worn value rather than only the slot name, so the rack
 * reads as the whole outfit while you are editing one part of it.
 *
 * @param slot the client reads this back as "which drawer to open"
 */
export const RackTab = ({ label, worn, color, on, ...rest } = {}) =>
  `<span${klass('rack-tab', on && 'on')}${attrs(rest)}>` +
    `<span${klass('swatch-chip', !color && 'none')}${color ? ` style="background:${color}"` : ''}></span>` +
    `<span${klass('rack-tab-text')}>` +
      `<span${klass('rack-tab-label')}>${label}</span>` +
      `<span${klass('rack-tab-worn')}>${worn}</span>` +
    `</span>` +
  `</span>`;

/** RackDrawer — one slot's worth of choices, open or put away. */
export const RackDrawer = ({ on, children, ...rest } = {}) =>
  `<div${klass('rack-drawer', on && 'open')}${attrs(rest)}>${join(children)}</div>`;

/**
 * WorldCard — the diegetic prompt.
 * Returns the card's contents; the container is owned by the renderer,
 * which positions and reveals it. It appears because of where the player
 * is standing, never because something pushed it to a feed.
 */
export const WorldCard = ({ kicker, body, quote, accept, decline, foot, panel } = {}) =>
  `<p${klass('card-kicker')}>${kicker}</p>` +
  `<p>${body}</p>` +
  `<p${klass('quiet')}>${quote}</p>` +
  ButtonRow([
    Button({ label: accept, variant: 'primary', panel }),
    Button({ label: decline, variant: 'ghost' }),
  ]) +
  `<p${klass('quiet', 'tiny', 'mb0')}>${foot}</p>`;

/**
 * Portrait — a lit silhouette holding the palette and the key light.
 * Placeholder for §12's two-tier character art; the real client swaps the
 * SVG for a painted plate behind this same interface.
 */
export const Portrait = (who) => {
  const warm = who === 'marta' ? '#f0a742' : '#ffb45e';
  const skin = who === 'marta' ? '#6b4a33' : '#7a5540';
  return `<svg viewBox="0 0 148 176" preserveAspectRatio="xMidYMid slice">` +
    `<defs><linearGradient id="bg${who}" x1="0" y1="0" x2="0" y2="1">` +
    `<stop offset="0%" stop-color="#241c17"/><stop offset="100%" stop-color="#100c0a"/></linearGradient>` +
    `<radialGradient id="key${who}" cx="26%" cy="22%" r="72%">` +
    `<stop offset="0%" stop-color="${warm}" stop-opacity=".38"/>` +
    `<stop offset="100%" stop-color="${warm}" stop-opacity="0"/></radialGradient></defs>` +
    `<rect width="148" height="176" fill="url(#bg${who})"/>` +
    `<rect width="148" height="176" fill="url(#key${who})"/>` +
    `<path d="M20 176 q10 -52 54 -58 q44 6 54 58 z" fill="#1b1512"/>` +
    `<path d="M20 176 q8 -48 42 -56 l6 56 z" fill="#241b15"/>` +
    `<ellipse cx="74" cy="82" rx="27" ry="33" fill="${skin}"/>` +
    `<path d="M47 78 q4 -34 27 -34 q23 0 27 34 q-6 -18 -27 -18 q-21 0 -27 18z" fill="#15100d"/>` +
    `<ellipse cx="74" cy="82" rx="27" ry="33" fill="#000" opacity=".34"/>` +
    `<path d="M52 62 q10 -20 22 -20 q12 0 22 20 q-8 46 -22 52" fill="${warm}" opacity=".16"/>` +
    `<path d="M51 70 q6 -28 23 -28" stroke="${warm}" stroke-width="2.4" fill="none" opacity=".85"/>` +
    `<path d="M48 92 q3 20 14 27" stroke="${warm}" stroke-width="1.8" fill="none" opacity=".5"/>` +
    `<circle cx="64" cy="82" r="2.4" fill="#0b0806"/><circle cx="85" cy="82" r="2.4" fill="#0b0806"/>` +
    `<path d="M66 98 q8 4 15 0" stroke="#0b0806" stroke-width="1.6" fill="none" opacity=".7"/>` +
    `</svg>`;
};
