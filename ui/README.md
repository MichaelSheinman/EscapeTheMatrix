# @etm/ui

The Escape the Matrix design system: the art bible encoded as tokens, plus the component set built
on top of it.

Consumed by `preview/` (static fixtures, no backend) and by `web/` (wired to the store). **Depends
on neither.** See `technical.md` §2 for why this is its own package.

```
tokens.css       the palette and the scales — the durable artifact
base.css         reset, document defaults, the handful of utilities
components.css   component styles; references the semantic layer only
layout.css       the client shell — full-bleed viewport, floating chrome, overlay
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
| `Token({ label, kind, size, on, act })` | kinds `cond` · `act` · `locked` |
| `Tokens(children)` | — |
| `Kw(word)` | — |
| `Ref(letter)` | — |
| `Group(children)` | — |

`Button` renders a `<span>`, not a `<button>`: the preview delegates clicks from the document root,
and a real button inside the overlay would take focus and steal Escape from the panel. **`web/`
should render a real `<button>`** — that difference is exactly why this is a component.

`Tag` is a *state*; if it can be clicked it is a `Button`. `Kw` is rule grammar and must never look
clickable.

**Hue does not type a term.** `cond` and `act` differ in weight, not colour: a condition is the
quiet half of a rule and an action is the committed one, and *which is which* is already said by the
keyword in front of it. Painting that a second time is what left every rule on screen two-coloured
for no added meaning. `Token`'s affordance likewise rides on `act`, not on `kind`: the same term is
inert inside a composed rule and live inside a picker.

`Ref` is a position in an expression that nothing has filled — the A / B / C a rule is discussed in
when it is being read rather than edited — and `Group` is the bracket around a pair, drawn in CSS
rather than typed into copy so the notation is one edit away from being anything else. Both are for
*displaying* structure; the live editor is `Editable` under Builder.

### Display
`Panel({ title, note, tag, body, bodyClass })` · `Stack(children)` · `Grid({ variant, children })`
(`two` · `wide-left`) · `Readout(children)` · `Stat({ value, unit, accent })` ·
`Metric({ name, sub, value, tone, pct, barTone, barOpacity, plain })` · `Bar({ pct, tone, opacity })` ·
`Sparkline({ points, target, targetLabel, mark, from, to })` · `LockedSlot({ name, why, dim })` ·
`Slots(children)` · `Notice({ text, tone })` (`warn` · `cool`) · `Tabs(items)` ·
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
`Rule({ when, about, expr, then, otherwise, meta, conflict, variant })` · `SceneHead({ tag, stages, note })` ·
`Dialogue({ who, name, role, children })` · `Speech(text, { size, variant })` ·
`Option({ lead, text, chosen })` · `Chip({ src, val, picked })` · `Chips(children)` ·
`Commit(children)` · `Portrait(who)`

`Rule.meta` takes strings, or `{ text, tone: 'danger' }` for the failing ones — one bad outcome is
noise, five is a diagnosable pattern.

`Rule` states its left-hand side two ways, and they are the same object at two ages. `when` is the
flat form, a list of conditions joined with AND. `about` + `expr` is the composed form: the
situation the rule answers, then an arbitrary expression the caller builds from `Token` / `Kw` /
`Group` / `Ref`. `variant: 'draft'` is a rule being composed rather than one in force.

### Builder
`Ask({ n, text, note })` · `Sentence(children)` · `Clause(children)` ·
`Editable({ kind, body, empty, menu, act })`
(kinds `cond` · `act` · `joint`) · `Num({ label, menu, act })` · `Menu({ items, extra })` ·
`Question({ n, ask, answer, state, active, act })`

The editor **is** the rule:

```
WHEN [something] THEN [do this], otherwise [do that]
```

Every part of the sentence opens a short list under the word it belongs to, so the rule never stops
being readable while it is being changed. There is no form, no preview of the result, and nothing to
submit. **Nothing here accepts free text**, which is the load-bearing constraint — a rule the player
typed is a rule the simulation cannot run and the player cannot diagnose afterwards — so every one
of these renders a closed, authored vocabulary.

`Clause` binds a keyword to the part it introduces so the two wrap together or not at all — a
sentence that breaks between THEN and what follows it has stopped being a sentence.

`Menu` holds the choices and nothing else — no explanatory line, and **no row standing in for a
choice that is not available yet**. Both make it taller than the sentence it is covering, which is
the one thing a menu anchored to a word cannot afford to be; and a term the player has not unlocked
turns up in the list the day it unlocks rather than sitting there blanked, so the surprise is not
spent in advance.

`Menu.extra` is what a part can *become* rather than what it can say: growing a second condition,
dropping one. The joint (`kind: 'joint'`) is the one piece of grammar that is also a control,
because it is the only word in the sentence that changes the logic.

**Four rules of restraint hold this section together**, and they are worth stating because the first
version of it broke all four:

1. **Hue never types a thing.** Nothing on this surface is coloured to say what kind of thing it is.
   The grammar already says that.
2. **One accent, spent on state that changes** — the tab you are on, the one primary action, the two
   states in the roster that want you (`Edited`, `Open`). Nothing at rest is amber.
3. **Interaction is weight and ground, not colour.** A hairline darkens; a transparent ground picks
   up a faint neutral wash.
4. **Nesting is a different *kind* of edge, never a second box.** The number inside a condition is
   underlined, not boxed — two frames inside each other read as clutter long before they read as
   depth.

**Editables nest.** A condition is clickable and the `Num` inside it is clickable on its own — which
condition, versus which threshold. The two grains must be distinguishable *before* the click, so the
outer hover stands down (`:has(.num:hover)`) while the pointer is on the inner one.

Every one of them acts through `act` — one `data-act="verb:argument"` string, read by the consumer
in one place, the same idiom `Swatch`'s `pick` uses. Which menu is open is state like any other, so
opening one and changing a word go through the same path.

`Swatch({ label, color, on, pick })` · `Swatches(children)` — one closed, authored choice each, sized
like a `Token` because they are the same kind of object: a small vocabulary the player picks from.
Omit `color` for the "none" option, which is a choice and has to look like one.

### Closet
`Wardrobe({ mirror, rack })` · `Mirror({ name, controls, note })` · `Rack({ tabs, drawers, note })` ·
`RackTab({ label, worn, color, on, slot })` · `RackDrawer({ on, drawer, children })`

The closet is the only screen whose subject is the player rather than a number about them, and it is
laid out like a fitting room: `Wardrobe` fills its panel so the figure stands in the middle of it and
the drawers sit on the bottom edge, within reach.

`Mirror` renders a `<canvas>` and nothing else — the consumer paints it, every frame, with the *same
routine that draws the figure in the street*. That is the load-bearing part: a coat that looked right
in a bespoke closet illustration and wrong from across the road would be a lie the closet told you.
The nameplate is positioned by the consumer too (`.mirror-name`, revealed with `.show`), because only
the drawing knows how tall you came out at this canvas size.

Every drawer is in the DOM and all but one carry no `on`, so changing which slot you are picking from
is a class toggle rather than a re-render and the figure above it never blinks. Same for a pick: a
`Swatch` moves a tick, and the figure was already wearing it. There is no preview, no confirm and no
save — the closet has no state of its own to hold, which is why closing it is walking away rather
than committing. `RackTab` carries the worn value as well as the slot name so the rack reads as the
whole outfit while you are editing one part of it.

### Shell
`StatPod({ glyph, value, note, label, tone, panel })` · `StatPods(children)` ·
`PlaceChip({ name, where, clock })` · `EnvChip({ label, name, rules, ruleTone })` ·
`PhoneButton({ count, key })` · `HelpButton({ key })` · `Toast({ kicker, title, body, action, panel })` ·
`ActionPrompt({ label, hint, key })` · `Keycap(k)` · `KeyBar(items)` · `KeyCard(groups)` ·
`Marker({ label })` · `NamePlate({ name })` · `PersonTip({ name, karma, note, hint })` · `Bark(text)` ·
`WorldCard({ kicker, body, quote, accept, decline, foot, panel })`

There is no rail and no top bar: the canvas is the whole screen, destinations are places you walk
into, and the shortcut for each one is a key. What survives as chrome is only what the world cannot
say for itself — your standing (`StatPod`), where you are (`PlaceChip`), which figure is you
(`NamePlate`), what is within reach (`ActionPrompt`), what arrived while you walked (`Toast`,
`PhoneButton`) and which key does it (`KeyBar`, `KeyCard`, `HelpButton`).

`NamePlate` is the only *permanent* label in the world attached to a person rather than a place.
Everyone else you learn to recognise by where they stand and what they wear, which is what makes
`Swatch` — the closet's picker — load-bearing rather than decorative.

`PersonTip` is the exception that keeps that true: it names somebody and reads out their Karma only
while you are pointing at them, so identifying a street stays something you do rather than something
the screen has already done. `hint` is the one thing you can do about them, and it lives on the tip
for the same reason the rest of it does — pointing is the question, the tip is the answer, and
nothing is labelled in advance. `Bark` is a line somebody says over their own head, with nothing
waiting on it; it is ambience, not `Dialogue`, and it must never be the only place a fact appears.

`Reply({ name, text })` is what somebody says back when you ask them one of their questions. It goes
directly under the `Option` that asked it rather than replacing the list, so a short closed set of
questions reads as a conversation instead of a menu that changed page — and it carries the
attribution, because an answer in this world belongs to a named person with a handle and a number,
never to the game. `Option` takes `ask` for that: one `data-ask="who:n"` string, read by the consumer
in one place, the same idiom `pick` and `act` use.

The shell is a library concern rather than an app concern because of §10.1: the canvas never
unmounts and panels are overlays above it, not routes that replace it. That constraint lives in
`.app` and the full-bleed `.viewport` so a consumer cannot accidentally violate it.

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
like a label, but uppercase micro-type is load-bearing elsewhere in the client (pod labels, panel
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
