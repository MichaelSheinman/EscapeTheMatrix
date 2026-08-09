import type { Screen } from '../App'
import { Toasts } from '../screens/Events'
import { useWorld } from '../world/react'
import { CITY_NAME } from './CityMap'

/**
 * The corner you reach for: what arrived, every shortcut, and the two buttons
 * that open them. One column, so nothing can overlap.
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
  const offer = useWorld((w) => w.events?.offer ?? null)

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
          className="corner-btn"
          onClick={() => onOpen('map')}
          title={`Where you are in ${CITY_NAME} (M)`}
        >
          <svg className="phone-glyph" viewBox="0 0 24 24" aria-hidden="true">
            <path
              d="M12 2.8 C 8.3 2.8 5.4 5.7 5.4 9.4 C 5.4 14.2 12 21.2 12 21.2 C 12 21.2 18.6 14.2 18.6 9.4 C 18.6 5.7 15.7 2.8 12 2.8 Z"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
            />
            <circle cx="12" cy="9.4" r="2.6" fill="currentColor" opacity="0.35" />
          </svg>
          <span className="corner-key">M</span>
        </button>
        <button
          className={`corner-btn help${card ? ' on' : ''}`}
          onClick={onCard}
          title="Every shortcut (?)"
        >
          <span className="help-glyph">?</span>
          <span className="corner-key">?</span>
        </button>
        <button
          className="corner-btn phone"
          onClick={() => onOpen('events')}
          title="What is happening (E)"
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
          {offer && <span className="phone-badge">1</span>}
          <span className="corner-key">E</span>
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
      { keys: ['M'], label: 'The city, and the ways out' },
      { keys: ['E'], label: 'What is happening' },
      { keys: ['C'], label: 'What is in front of you' },
      { keys: ['A'], label: 'The people you live among' },
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
      { keys: ['J'], label: 'Work, and the ladder' },
      { keys: ['K'], label: 'Where you stand' },
      { keys: ['L'], label: 'The boards' },
    ],
  },
  {
    title: 'View',
    items: [
      { keys: ['Q'], label: 'Effects tier' },
      { keys: ['Esc'], label: 'Back to the world' },
      { keys: ['?'], label: 'This card' },
    ],
  },
]
