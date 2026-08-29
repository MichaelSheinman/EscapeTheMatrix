# Escape the Matrix

Life-simulator game. Go backend (`cmd/`, `internal/`), authored content in
`content/*.yaml`, React client in `web/`. `DESIGN.md` is the living spec.

The full rulebook, with the incidents behind each rule, is
`Documents/Guidelines.md`. Read that file before UI work. The rules below are
the always-on distillation; Guidelines.md wins on any conflict.

## Workflow

- Do not start or kill the server. One is already running on :8080 in another
  terminal; test against that, and ask Michael to restart when needed.
- Do not write unit tests. Verify every feature end-to-end in the browser: a
  backend feature counts as done only when a user path in the UI reaches the
  result. A test written to debug something is deleted with the bug.
- Do not implement anything that was not asked for. A ticket that needs a
  demonstration vehicle gets the smallest possible one, clearly marked as
  scaffolding.
- Things need to happen. V1 is about building many visible pathways and
  opportunities so the player is never bored. Keep features reachable and
  visible while building; locking comes in later versions.

## Content

- Content imitates life. Authored content imitates the real-world
  counterpart, the literary register is banned. All player-visible 
  prose in `content/*.yaml` goes through the `write-content` skill — never composed inline, not even a one-sentence
  tweak. Numbers the mechanics own (pay, hours, shift times) are injected by
  the UI, never written into prose.
- Challenge text lives in the YAML, never in a React component. Components lay
  out and render; they supply no sentences of their own.
- No fluff text. If a sentence can be cut without losing meaning, cut it. Do
  not introduce extra views for text.
- Do not teach users. The game makes the player think by living through
  interactions; the UI never analyzes choices or states outcomes in advance.
- Do not narrate your own discoveries. A sentence that only parses as a diff
  ("you no longer need to…", "unlike before") belongs in the commit message,
  not the game.
- Never show locked features. Features are either usable or hidden.

## UI

- Avoid multi-column layouts of two disconnected views. The exception is a
  connected pair — a list beside the item opened from it (list on the left),
  or a navbar for the content beside it. See `.split` in ui/components.css.

## Writing (comments, prose, descriptions)

- Say what something is, never what it is not. The most concise description of
  the thing, and nothing else.
- No analogies or metaphors in comments, and no complex words. Describe the
  thing plainly.
- No historical context in comments. Describe what the code is, not what it
  used to be or what the change did.
