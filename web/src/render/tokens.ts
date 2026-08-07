/*
 * The design system, read off the document.
 *
 * `@etm/ui` owns the palette (`ui/tokens.css`), and its rule is that nothing
 * outside that file may introduce a colour and nothing may reach past the
 * semantic layer into a primitive. A canvas cannot use CSS, so the choice is
 * between literal hexes in the renderer — the art bible with two homes, which
 * `ui/README.md` lists as a known gap — and reading the same custom properties
 * off the document once at boot. This is the second.
 *
 * Every name below is a semantic role, never a primitive. A role the world
 * needs and the design system does not have is a missing role, and belongs in
 * `tokens.css` rather than here.
 */

/** The roles the renderer draws with, and what to use if one is missing. */
const roles = {
  sky: ['--sky', 0xcfe0ee],
  ground: ['--bg-sunken', 0xefe7d9],
  horizon: ['--border-default', 0xd8cdb9],
  paper: ['--bg-surface', 0xfffdf8],
  accent: ['--bg-accent-solid', 0xb26a22],
  info: ['--bg-info-solid', 0x3a6ba6],
  text: ['--text-default', 0x2f2820],
  muted: ['--text-muted', 0x978b77],
} as const satisfies Record<string, readonly [string, number]>

// `-readonly` because the table above is a const assertion and the palette
// built from it is filled in a loop.
export type Palette = { -readonly [K in keyof typeof roles]: number }

export type Tokens = {
  color: Palette
  /** The UI font stack, so a number on the canvas is set in the same face. */
  font: string
}

/**
 * Resolve the tokens once.
 *
 * Once rather than per frame because these are constants of the build — the
 * palette is not themed at runtime, and `getComputedStyle` is a layout read
 * that has no business in a frame loop.
 */
export function readTokens(): Tokens {
  const style = getComputedStyle(document.documentElement)
  const color = {} as Palette

  for (const [name, [property, fallback]] of Object.entries(roles)) {
    color[name as keyof Palette] = parseColor(style.getPropertyValue(property)) ?? fallback
  }

  return {
    color,
    font: style.getPropertyValue('--font-sans').trim() || 'system-ui, sans-serif',
  }
}

/**
 * A CSS colour as the number Pixi wants, or null if it is not one this
 * understands.
 *
 * Custom properties come back computed — `var()` already substituted — so what
 * arrives here is whatever `tokens.css` wrote at the bottom of the chain. Hex
 * and `rgb()` cover the palette; alpha is deliberately dropped, because opacity
 * on the canvas belongs to the object being drawn rather than to its colour.
 */
function parseColor(value: string): number | null {
  const text = value.trim()
  if (!text) return null

  const hex = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(text)
  if (hex) {
    const digits = hex[1]
    if (digits.length === 3) {
      // #abc is #aabbcc.
      const [r, g, b] = [...digits].map((d) => parseInt(d + d, 16))
      return (r << 16) | (g << 8) | b
    }
    return parseInt(digits, 16)
  }

  const rgb = /^rgba?\(([^)]+)\)$/i.exec(text)
  if (rgb) {
    const parts = rgb[1].split(/[\s,/]+/).filter(Boolean).map(Number)
    if (parts.length < 3 || parts.slice(0, 3).some(Number.isNaN)) return null
    const [r, g, b] = parts
    return ((r & 0xff) << 16) | ((g & 0xff) << 8) | (b & 0xff)
  }

  return null
}
