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
const attrs = ({ panel, move, when, go, scene, style, title } = {}) => cx(
  panel ? `data-panel="${panel}"` : '',
  move ? `data-move="${move}"` : '',
  when ? `data-when="${when}"` : '',
  go ? `data-go="${go}"` : '',
  scene ? `data-scene="${scene}"` : '',
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
 */
export const Token = ({ label, kind = 'cond', size } = {}) =>
  `<span${klass('token', kind, size === 'small' && 'small')}>${label}</span>`;

/** A wrapped set of tokens. */
export const Tokens = (children) => `<div${klass('tokens')}>${join(children)}</div>`;

/** Kw — rule grammar (WHEN / AND / THEN). Must never look clickable. */
export const Kw = (word) => `<span${klass('kw')}>${word}</span>`;


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

/** Footnote — preview-time annotation on what a screen demonstrates. */
export const Footnote = (text) =>
  `<div${klass('footnote')}><b>Demonstrating.</b> ${text}</div>`;

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
 * @param when  array of condition labels; joined with AND
 * @param then  the action label
 * @param meta  array of strings, or { text, tone } for the failing ones
 */
export const Rule = ({ when = [], then, meta = [], conflict, ...rest } = {}) => {
  const conds = when.map((c, i) =>
    (i ? Kw('and') : '') + Token({ label: c, kind: 'cond' })
  ).join('');
  const metaLine = meta.length
    ? `<div${klass('rule-meta')}>` + meta.map((m) =>
        typeof m === 'string' ? `<span>${m}</span>`
          : `<span${klass(m.tone === 'danger' && 'warn-text')}>${m.text}</span>`
      ).join('') + `</div>`
    : '';
  return `<div${klass('rule', conflict && 'conflict')}${attrs(rest)}>` +
    `<div${klass('rule-line')}>${Kw('when')}${conds}${Kw('then')}` +
    Token({ label: then, kind: 'act' }) + `</div>${metaLine}</div>`;
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

/** Option — an authored, committable reading. Never marked correct. */
export const Option = ({ lead = 'Commit to this', text, chosen } = {}) =>
  `<div${klass('option', chosen && 'chosen')}>` +
  `<div${klass('option-lead')}>${lead}</div>` +
  `<div${klass('option-text')}>${text}</div></div>`;

/** Chip — a piece of the player's own evidence, selectable in a diagnosis. */
export const Chip = ({ src, val, picked } = {}) =>
  `<div${klass('chip', picked && 'picked')}>` +
  `<span${klass('chip-src')}>${src}</span>` +
  `<span${klass('chip-val')}>${val}</span></div>`;

/** A wrapped set of Chips. */
export const Chips = (children) => `<div${klass('chips')}>${join(children)}</div>`;

/** Commit — the confirmation step below a set of Chips. */
export const Commit = (children) => `<div${klass('commit')}>${join(children)}</div>`;

/* ═════════════════════════════════════════════════════════════ SHELL */

/**
 * RailLink — one destination in the navigation rail.
 * @param note   a second line, for a place you cannot currently reach
 * @param locked stacks label over note and greys the whole row
 */
export const RailLink = ({ label, glyph, note, locked, ...rest } = {}) => {
  const inner = `<span${klass('rail-glyph')}>${glyph}</span> ${label}`;
  return `<a${klass(locked && 'locked')}${attrs(rest)} href="#">` +
    (locked ? `<span${klass('rail-row')}>${inner}</span>` : inner) +
    (note ? `<span${klass('rail-note')}>${note}</span>` : '') + `</a>`;
};

/** Marker — the label-and-dot pinned over a point in the world. */
export const Marker = ({ label } = {}) =>
  `<span${klass('marker-label')}>${label}</span><span${klass('marker-dot')}></span>`;

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
