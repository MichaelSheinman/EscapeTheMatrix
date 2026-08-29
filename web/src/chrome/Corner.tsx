import type { Screen } from '../App'
import { Toasts } from '../screens/Events'
import { useWorld } from '../world/react'

/**
 * The corner you reach for: what arrived, every shortcut, and the button that
 * opens them. One column, so nothing can overlap.
 */
export function Corner({
  onOpen,
  card,
  onCard,
}: {
  onOpen: (screen: Screen) => void
  card: boolean
  onCard: () => void
}) {
  const waiting = useWorld((w) => w.conflicts?.waiting ?? null)

  return (
    <div className="corner">
      <Toasts onOpen={onOpen} />

      <div className={`keycard${card ? ' open' : ''}`}>
        {KEYCARD.map((group) => (
          <div className="keycard-group" key={group.title}>
            <p className="keycard-title">{group.title}</p>
            {group.items.map((item) => (
              <div className="keycard-row" key={item.label}>
                <span className="keycard-keys">
                  {item.keys.map((k) => (
                    <kbd className="keycap" key={k}>
                      {k}
                    </kbd>
                  ))}
                </span>
                <span className="keycard-label">{item.label}</span>
              </div>
            ))}
          </div>
        ))}
        <p className="keycard-note">
          Nothing here is the only way in. The board, the closet, the computer and the bed are in
          your room; the ways out of a place are its doors; and a challenge finds you where it
          happens.
        </p>
      </div>

      <div className="corner-btns">
        <button
          className={`corner-btn help${card ? ' on' : ''}`}
          onClick={onCard}
          title="Every shortcut (?)"
        >
          <span className="help-glyph">?</span>
          <span className="corner-key">?</span>
        </button>
        {/* The phone opens the phone. It used to open "what is happening",
            which was the metaphor before there was a device — and once there
            was one, a phone-shaped button that opened something else was the
            only genuinely misleading control in the shell. What arrives from
            the world still arrives: the toast above, and the key bar reading
            differently for E. */}
        <button
          className="corner-btn phone"
          onClick={() => onOpen('phone')}
          title="Your phone (T)"
        >
          <svg className="phone-glyph" viewBox="0 0 24 24" aria-hidden="true">
            <rect
              x="6.5"
              y="2.5"
              width="11"
              height="19"
              rx="2.6"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
            />
            <rect x="8.6" y="5.4" width="6.8" height="11.2" rx="0.9" fill="currentColor" opacity="0.16" />
            <line
              x1="10.4"
              y1="4"
              x2="13.6"
              y2="4"
              stroke="currentColor"
              strokeWidth="1.2"
              strokeLinecap="round"
            />
            <circle cx="12" cy="19.1" r="0.95" fill="currentColor" />
          </svg>
          {/* The badge counts to one and only ever will: nothing new comes up
              with these people until the last thing is dealt with, so it is a
              light saying "somebody is waiting" rather than a tally. */}
          {waiting && <span className="phone-badge">1</span>}
          <span className="corner-key">T</span>
        </button>
      </div>
    </div>
  )
}

const KEYCARD = [
  {
    title: 'Moving',
    items: [
      { keys: ['←', '↑', '→', '↓'], label: 'Walk' },
      { keys: ['click'], label: 'Walk to that spot' },
      { keys: ['O'], label: 'Use the nearest thing — a door opens by walking in too' },
    ],
  },
  {
    title: 'Where you are',
    items: [
      { keys: ['E'], label: 'What is happening' },
      { keys: ['C'], label: 'What is in front of you' },
      { keys: ['T'], label: 'Your phone — what has passed with them, and what was said' },
    ],
  },
  {
    title: 'You',
    items: [
      { keys: ['S'], label: 'Your life' },
      { keys: ['H'], label: 'Your body' },
      { keys: ['R'], label: 'Your routines' },
      { keys: ['D'], label: 'What you think happened' },
      { keys: ['P'], label: 'Playbook' },
      { keys: ['J'], label: 'Work, and what else is going' },
      { keys: ['K'], label: 'Where you stand' },
      { keys: ['L'], label: 'The boards' },
    ],
  },
  {
    title: 'View',
    items: [
      { keys: ['Esc'], label: 'Back to the world' },
      { keys: ['?'], label: 'This card' },
    ],
  },
]
