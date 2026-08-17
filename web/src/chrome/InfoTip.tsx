/**
 * InfoTip — a trigger that states one fact about the thing beside it.
 *
 * The React twin of `ui/index.js`'s `InfoTip`, and deliberately the same markup:
 * the class names, the `data-place` and the arrow are all the library's, so both
 * are painted by one block of CSS and neither can drift.
 *
 * It holds no state. Open is `:hover` and `:focus-within` in CSS, which is
 * the library's rule for every hover in it — a component with an `open` boolean
 * would behave differently from the gallery's copy the first time somebody
 * touched either.
 *
 * It is a `<button>` rather than a bare glyph because the fact has to be
 * reachable without a pointer. That is the whole reason this exists instead of
 * a `title=` attribute, which no keyboard opens and no phone shows.
 *
 * It states a fact, never an instruction. A sentence that only parses as
 * advice is a coach mark, and a coach mark is the UI doing the player's
 * thinking for them.
 */
export function InfoTip({
  text,
  place = 'top',
  label = 'More information',
}: {
  text: string
  place?: 'top' | 'right' | 'bottom' | 'left'
  label?: string
}) {
  return (
    <span className="infotip" data-place={place}>
      <button
        type="button"
        className="infotip-trigger"
        aria-label={label}
        // The trigger opens a fact and commits nothing. Without this it is a
        // submit inside any form it lands in, and inside the job board it is a
        // button nested in a row that is itself a button.
        onClick={(e) => e.preventDefault()}
      >
        i
      </button>
      <span className="infotip-bubble" role="tooltip">
        {text}
      </span>
    </span>
  )
}
