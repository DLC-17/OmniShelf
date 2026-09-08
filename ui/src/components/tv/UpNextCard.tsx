import { useState } from 'react'
import type { UpNextCard as UpNextCardData } from '../../hooks/useUpNext'
import EpisodeList from './EpisodeList'
import Poster from './Poster'

/** "S03E07" — TV Time style episode code. */
function episodeCode(season: number, number: number): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `S${pad(season)}E${pad(number)}`
}

interface UpNextCardProps {
  entry: UpNextCardData
  onMarkWatched: (episodeId: number) => void
}

/**
 * One Up Next dashboard card:
 * 2-column layout:
 * - Left: 30% width cover art scaling height to container.
 * - Right: show name, episode code & title, and air date.
 */
export default function UpNextCard({ entry, onMarkWatched }: UpNextCardProps) {
  const { show, episode } = entry
  const watched = entry.optimisticWatched === true
  const code = episodeCode(episode.season, episode.number)
  const [expanded, setExpanded] = useState(false)

  return (
    <li className={watched ? 'card is-dim' : 'card'}>
      <div className="show-card-row">
        {/* Left Column: 30% width cover art scaling height to container */}
        <div
          className="show-card-poster"
          onClick={() => setExpanded((v) => !v)}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              setExpanded((v) => !v)
            }
          }}
          title={`${expanded ? 'Hide' : 'Show'} episodes for ${show.title}`}
          style={{ cursor: 'pointer' }}
        >
          <Poster
            posterPath={show.posterPath}
            title={show.title}
            style={{ width: '100%', height: '100%', objectFit: 'cover' }}
          />
        </div>

        {/* Right Column: Show Name, Episode & Title, Air Date */}
        <div className="show-card-content">
          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '0.5rem', width: '100%' }}>
            <button
              type="button"
              className="show-toggle"
              aria-expanded={expanded}
              aria-label={`${expanded ? 'Hide' : 'Show'} episodes for ${show.title}`}
              onClick={() => setExpanded((v) => !v)}
              style={{ padding: 0, width: '100%', textAlign: 'left', background: 'none' }}
            >
              <h3 style={{ margin: '0 0 0.25rem', fontSize: '1.05rem', lineHeight: 1.25 }}>
                {show.title} <span className="show-caret" aria-hidden="true">{expanded ? '▾' : '▸'}</span>
              </h3>
              <p style={{ margin: '0 0 0.25rem', fontWeight: 550, color: 'var(--text)' }}>
                <span className="badge" style={{ marginRight: '0.4rem', fontSize: '0.75rem', padding: '0.1rem 0.45rem' }}>
                  {code}
                </span>
                {episode.title !== '' && <span>{episode.title}</span>}
              </p>
              {episode.airDate !== null && (
                <p className="meta" style={{ fontSize: '0.82rem', margin: 0 }}>
                  Aired {episode.airDate}
                </p>
              )}
            </button>

            <button
              type="button"
              className="check"
              aria-label={`Mark ${show.title} ${code} watched`}
              aria-pressed={watched}
              disabled={watched}
              onClick={(e) => {
                e.stopPropagation()
                onMarkWatched(episode.id)
              }}
              style={{ flexShrink: 0, marginTop: '0.1rem' }}
            >
              &#10003;
            </button>
          </div>
        </div>
      </div>
      {expanded && <EpisodeList showId={show.id} />}
    </li>
  )
}
