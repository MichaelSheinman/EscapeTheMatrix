import type { ReactNode } from 'react'

/*
 * The overlay shell every screen stands in: scrim, panel, the Esc button, a
 * page head, and the body. Markup only, deliberately — a screen's behaviour
 * (what it loads when it opens, what it holds while it is closed) is the
 * screen's own, and this file must never grow an opinion about it.
 *
 * Closing one does not unmount anything the renderer owns — press Escape and
 * the street is still there (§10.1). The screens stay mounted behind `open`,
 * which is also what lets them keep their tab-local state while closed.
 */
export function Overlay({
  open,
  onClose,
  title,
  sub,
  onBodyClick,
  children,
}: {
  open: boolean
  onClose: () => void
  title: ReactNode
  sub?: ReactNode
  /** For screens whose body click closes an in-place menu. */
  onBodyClick?: () => void
  children: ReactNode
}) {
  return (
    <div className={`overlay${open ? ' open' : ''}`}>
      <div className="overlay-scrim" onClick={onClose} />

      <div className="overlay-panel">
        <button className="overlay-close" onClick={onClose}>
          <kbd className="keycap">Esc</kbd> back to the world
        </button>

        <div className="page-head">
          <h1 className="page-title">{title}</h1>
          {sub && <p className="page-sub">{sub}</p>}
        </div>

        <div className="overlay-body" onClick={onBodyClick}>
          {children}
        </div>
      </div>
    </div>
  )
}
