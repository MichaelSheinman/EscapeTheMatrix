# @etm/ui

The Escape the Matrix design system: the art bible encoded as tokens, plus the component set built
on top of it.

Consumed by `preview/` (static fixtures, no backend) and by `web/` (wired to the store). **Depends
on neither.** See `technical.md` §2 for why this is its own package.

```
tokens.css       the palette and the scales — the durable artifact
base.css         reset, document defaults, the handful of utilities
components.css   component styles; references the semantic layer only
layout.css       the client shell — rail, HUD, viewport chrome, overlay
index.css        entry point; @imports the four above in order
index.js         component functions
gallery.html     every component in every state
```

Open the gallery with `npm run gallery` in this directory, then
<http://localhost:8898/gallery.html>.

---

## The dependency rule

`ui/` imports nothing from `web/` or `preview/`. It holds no game logic, no store access, and no
fetching. **It renders what it is handed.** Copy lives in the caller — and eventually in `content/`
(§5.2), not here.

The one-way arrow is what stops the playground and the real client from drifting apart, which is
what happens when a prototype is styled separately and later "ported".

---

## Tokens: two layers

The split is the whole point of the package.

**1. Primitives** — the palette, named by hue and step: `--warm-50…800`, `--amber-200…700`,
`--blue-200/600`, `--green-600`, `--rust-500/600`. This is `HighLevelOverview.md` §4's LOCKED art
bible, written down. Nothing outside `tokens.css` may name a primitive.

**2. Semantics** — roles: `--bg-surface`, `--border-muted`, `--text-subtle`, `--ink-plot`.
Components reference only these.

```css
/* wrong — reaches through the semantic layer */
.thing { border-color: var(--warm-500); }

/* right */
.thing { border-color: var(--border-default); }
```

If a component needs a colour that has no role, **the role is missing** — add it to `tokens.css`
rather than reaching through. This is what makes "soften the dividers but keep the card borders" a
one-line change instead of a 21-site one.

The same applies to `index.js`: variants are semantic, never presentational. `variant: 'danger'`,
never `variant: 'red'`. A variant named after the paint is wrong the first time the paint changes.

---

## Components

Every function returns an HTML string. That is an interim shape — it matches how `preview/` renders
today, and each one is a direct translation target for a JSX component in `web/`. **The contract
(names, option keys, variant vocabulary) is what survives the port; the string building is not.**

Text arguments are trusted authored HTML: entities and inline markup pass through unescaped, because
the copy is repo-authored and often contains `Token()` spans. Never hand these player input.

### Primitives
| | Variants |
| --- | --- |
| `Button({ label, variant, size })` | `default` · `primary` · `ghost` · `spent` — sizes `default` · `small` |
| `ButtonRow(children)` | — |
| `Tag({ label, variant })` | `default` · `accent` · `danger` · `success` · `dim` |
| `Token({ label, kind, size })` | kinds `cond` · `act` · `locked` |
| `Tokens(children)` | — |
| `Kw(word)` | — |

`Button` renders a `<span>`, not a `<button>`: the preview delegates clicks from the document root,
and a real button inside the overlay would take focus and steal Escape from the panel. **`web/`
should render a real `<button>`** — that difference is exactly why this is a component.

`Tag` is a *state*; if it can be clicked it is a `Button`. `Kw` is rule grammar and must never look
clickable.

### Display
`Panel({ title, note, tag, body, bodyClass })` · `Stack(children)` · `Grid({ variant, children })`
(`two` · `wide-left`) · `Readout(children)` · `Stat({ value, unit, accent })` ·
`Metric({ name, sub, value, tone, pct, barTone, barOpacity, plain })` · `Bar({ pct, tone, opacity })` ·
`Sparkline({ points, target, targetLabel, mark, from, to })` · `LockedSlot({ name, why, dim })` ·
`Slots(children)` · `Notice({ text, tone })` (`warn` · `cool`) · `Footnote(text)` · `Tabs(items)` ·
`Note(text, opts)` · `Prose(text, opts)` · `Divider()`

`Metric` separates `tone` (the value) from `barTone` (the bar) because a neutral number can sit on
an alarming trend. `LockedSlot`'s `why` states the *missing instrument*, never the hidden value.

### Rows
`LogRow({ time, title, detail, trailing, dim })` · `BoardRow({ rank, name, where, value, tone, variant })` ·
`LadderRow({ tier, title, detail, trailing, variant })` · `DecayRow({ label, pct, amount })` ·
`Posting({ role, where, pay, payNote, hours, reqs, foot, action })`

`Posting`'s `reqs` states are `met` · `unmet` · `hidden` — the last is a hidden factor gating the
role, shown as a blanked slot that is simply not high enough.

### Composites
`Rule({ when, then, meta, conflict })` · `SceneHead({ tag, stages, note })` ·
`Dialogue({ who, name, role, children })` · `Speech(text, { size, variant })` ·
`Option({ lead, text, chosen })` · `Chip({ src, val, picked })` · `Chips(children)` ·
`Commit(children)` · `Portrait(who)`

`Rule.meta` takes strings, or `{ text, tone: 'danger' }` for the failing ones — one bad outcome is
noise, five is a diagnosable pattern.

### Shell
`RailLink({ label, glyph, note, locked, go, scene, panel })` · `Marker({ label })` ·
`WorldCard({ kicker, body, quote, accept, decline, foot, panel })`

The shell is a library concern rather than an app concern because of §10.1: the canvas never
unmounts and panels are overlays above it, not routes that replace it. That constraint lives in the
`.app` grid so a consumer cannot accidentally violate it.

---

## Known gaps

The extraction itself was done at **visual parity** — the preview rendered pixel-for-pixel what it
did before, so the refactor was verifiable against the previous build. The curvature pass below was
then applied on top as a separate change, and it touched `tokens.css` plus four lines of
`components.css`. That ratio is the argument for the package.

*Done:* the radius scale now steps by role (card 10 → inner 7 → control 6 → chip 5), and `Tag` uses
`--radius-pill` so a state reads as a pill and a container reads as a card. Meters and stage dots
are pill-capped too.

**Tag is still uppercase and letter-spaced.** Now a pill, but the typography is unchanged —
`POOR`, `LOCKED`, `DECLINED`. Sentence case with no tracking would read more like a status and less
like a label, but uppercase micro-type is load-bearing elsewhere in the client (rail groups, panel
titles), so changing it for `Tag` alone is a deliberate break rather than a cleanup. Two lines in
`.tag` when wanted.

**The type scale has 13 steps.** Named by role in `tokens.css` (`--fs-micro` … `--fs-display`) at
their existing values. Several are within 0.5px of each other (`--fs-micro`/`--fs-label`,
`--fs-caption`/`--fs-meta`, `--fs-small`/`--fs-tiny`) and are accidents rather than decisions. A
sensible collapse is to 7.

**Six row components are one component.** `LogRow`, `BoardRow`, `LadderRow`, `DecayRow`, `Posting`
and the inbox rows are near-identical grids: lead / title + detail / trailing. They are kept
separate to preserve current rendering. Consolidating them behind one `Row` with a lead slot is
what makes an avatar column possible everywhere at once.

**There is no Avatar.** Rows are anchored by a mono timestamp, which is the least interesting thing
in them. `Portrait` already exists for dialogue; a 32px circular variant plus a rounded-square form
for institutions is the missing primitive.

**The canvas has its own colours.** `preview/src/app.js` and `render.js` draw to a 2D context and
carry literal hexes (`#fff0cc`, `#8ec4de`, `#22303f`, …). They cannot read CSS variables without
`getComputedStyle`, so the art bible currently has two homes. Worth a `tokens.js` mirror export.

**Swatch count.** §4 specifies "10–12 swatches total". This package defines 18 across five hue
families. The §4 constraint reads as governing painted *scenes* ("no scene introduces a hue outside
it") rather than UI chrome, and the count matches what the client already used — but it is a
genuine tension with a LOCKED section and needs a call.

---

## Porting to `web/`

1. `tokens.css`, `base.css`, `components.css`, `layout.css` move **unchanged**. Vite inlines the
   `@import` chain.
2. Each `index.js` function becomes a `.tsx` component with the same name and the same props. The
   option objects are already the prop shapes.
3. `Button` starts rendering a real `<button>`; `data-panel` becomes an `onClick`.
4. `gallery.html` becomes Storybook if the story count justifies it — the states are already
   enumerated there and transcribe directly.

Nothing in `preview/` needs to survive that. That is the design.
