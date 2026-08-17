/*
 * The things in the flat, drawn as inline SVG and keyed by the same string the
 * server sends as an option key. That key is the join: `salvage.yaml` names an
 * item `dog`, the plan offers `dog` as a choice, `markers.gen.ts` pins a marker
 * carrying `dog`, and this file knows what a dog looks like.
 *
 * Art is on the client (§10.3). The server says which object and never what it
 * looks like, so a redraw changes this file and a rewrite of the prose changes
 * `salvage.yaml`. A picture chosen by matching on the item's description would
 * stop working the first time somebody edited a word.
 *
 * A key with nothing drawn for it falls back to a plain parcel, so a new item
 * can be authored in YAML and played the same afternoon with the picture
 * arriving later.
 *
 * They sit at 20–40px in a bag slot and on a choice card, over a painted world
 * with a strong silhouette style, so each is a couple of paths that read at a
 * glance: the dog is a dog because of its ears and tail. They inherit
 * `currentColor`, so a slot can dim one without a second asset.
 */

/** The one size that matters: everything below is drawn in a 24×24 box. */
const BOX = 24

export type ThingProps = {
  /** The item key, as authored in salvage.yaml and sent as an option key. */
  item: string
  /** Rendered size in CSS pixels. Slots use 28, choice cards 34. */
  size?: number
  title?: string
}

/**
 * One thing, drawn.
 *
 * `aria-hidden` unless it is given a title: in every place this is used the
 * item's name is already beside it in text, and a screen reader announcing
 * "dog, dog" is worse than one announcing it once.
 */
export function Thing({ item, size = 28, title }: ThingProps) {
  const paths = THINGS[item] ?? THINGS.parcel
  return (
    <svg
      viewBox={`0 0 ${BOX} ${BOX}`}
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      role={title ? 'img' : undefined}
      aria-hidden={title ? undefined : true}
      aria-label={title}
    >
      {title && <title>{title}</title>}
      {paths}
    </svg>
  )
}

/** Whether this build has a picture for an item, for a caller that cares. */
export function drawn(item: string): boolean {
  return item in THINGS
}

/*
 * The nine, plus the fallback.
 *
 * Written as JSX rather than path strings so each one is readable and editable
 * in place — these are art, and art nobody can see the shape of in the source is
 * art nobody adjusts.
 */
const THINGS: Record<string, React.ReactNode> = {
  // A dog, sitting, seen from the side. Ears and tail do the work.
  dog: (
    <>
      <path d="M7 20v-5.5a3.5 3.5 0 0 1 3.5-3.5H14" />
      <path d="M14 11a4 4 0 0 0 4-4V5l-2.2 1.6A3 3 0 0 1 14 7h-1" />
      <path d="M13 7a3 3 0 0 0-3 3" />
      <path d="M18 5.2 19.6 4 20 6.4" />
      <path d="M14 11.2V20" />
      <path d="M7 20h2M14 20h2" />
      <path d="M7 15c-1.6.4-2.6 1.8-2.6 3.4" />
      <circle cx="17.4" cy="6.6" r=".7" fill="currentColor" stroke="none" />
    </>
  ),

  // A sealed urn: lidded, footed, with the tape across the lid.
  urn: (
    <>
      <path d="M8 8h8" />
      <path d="M9 8c-.6 3 .4 4.8.4 7.2 0 1.8-.6 2.9-.6 3.8h6.4c0-.9-.6-2-.6-3.8 0-2.4 1-4.2.4-7.2" />
      <path d="M7.5 19h9" />
      <path d="M10 8V6.6a2 2 0 0 1 4 0V8" />
      <path d="M9.4 11.6h5.2" />
    </>
  ),

  // A ziplock folder of papers, with the seal along the top.
  papers: (
    <>
      <rect x="5" y="5.5" width="14" height="14" rx="1.2" />
      <path d="M5 9h14" />
      <path d="M8.5 12.5h7M8.5 15.5h4.5" />
      <path d="M10.5 5.5v3.5" />
    </>
  ),

  // A shoebox with the lid slightly off, and a print sticking out.
  photos: (
    <>
      <path d="M4.5 10.5h15v8.2a.8.8 0 0 1-.8.8H5.3a.8.8 0 0 1-.8-.8z" />
      <path d="M4 8.4h16v2.1H4z" />
      <path d="M13.6 8.4 15 4.6l3.4 1.2-1 3" />
      <circle cx="15.9" cy="6.4" r=".8" />
    </>
  ),

  // A phone, face down, with the cable still on it.
  phone: (
    <>
      <rect x="7.5" y="3.5" width="9" height="15" rx="1.6" />
      <path d="M10.6 6.2h2.8" />
      <path d="M12 15.6h.01" />
      <path d="M12 18.5v1.2a1.3 1.3 0 0 0 1.3 1.3H16" />
    </>
  ),

  // A worn wallet, folded, with a card standing out of it.
  wallet: (
    <>
      <path d="M4.5 8.2h15a.8.8 0 0 1 .8.8v8.6a.8.8 0 0 1-.8.8h-15a.8.8 0 0 1-.8-.8V9a.8.8 0 0 1 .8-.8z" />
      <path d="M16 12.2h4.3v3H16a1.5 1.5 0 0 1 0-3z" />
      <path d="M6 8.2 14.4 5l1.6 3.2" />
    </>
  ),

  // A roll of notes, banded.
  cash: (
    <>
      <rect x="3.5" y="8" width="17" height="8" rx="1.2" />
      <path d="M11 8v8" />
      <path d="M13 8v8" />
      <circle cx="7.2" cy="12" r="1.8" />
      <path d="M17 10.4v3.2" />
    </>
  ),

  // Cut flowers, out of the jug.
  flowers: (
    <>
      <path d="M12 20v-7" />
      <path d="M12 13c-2.4-.6-3.8-2-4.2-4.2" />
      <path d="M12 13c2.4-.6 3.8-2 4.2-4.2" />
      <circle cx="12" cy="6.4" r="2.2" />
      <circle cx="7.2" cy="8.2" r="1.5" />
      <circle cx="16.8" cy="8.2" r="1.5" />
      <path d="M9.6 20h4.8" />
    </>
  ),

  // The armchair, three-quarter, because it is a chair and it is a problem.
  chair: (
    <>
      <path d="M6.5 12V8.2a2 2 0 0 1 2-2h7a2 2 0 0 1 2 2V12" />
      <path d="M5 12.4a1.6 1.6 0 0 1 3.2 0v2.2h7.6v-2.2a1.6 1.6 0 0 1 3.2 0V17H5z" />
      <path d="M6.4 17v2.4M17.6 17v2.4" />
    </>
  ),

  // Anything this build has not drawn: a parcel, plainly.
  parcel: (
    <>
      <path d="M4.5 8.6 12 5l7.5 3.6v7.8L12 20l-7.5-3.6z" />
      <path d="M4.5 8.6 12 12.2l7.5-3.6" />
      <path d="M12 12.2V20" />
    </>
  ),
}
