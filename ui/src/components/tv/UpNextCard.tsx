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
 * One Up Next dashboard card: poster, show title, next episode label and the
 * one-tap watch checkmark. Clicking the show opens the full episode picker so
 * the user can mark any episode watched, rewatch, or catch up in bulk.
 */
export default function UpNextCard({ entry, onMarkWatched }: UpNextCardProps) {
  const { show, episode } = entry
  const watched = entry.optimisticWatched === true
  const code = episodeCode(episode.season, episode.number)
  const label = episode.title === '' ? code : `${code} · ${episode.title}`
  const [expanded, setExpanded] = useState(false)

  return (
    <li className={watched ? 'card is-dim' : 'card'}>
      <div className="card-row">
        <Poster posterPath={show.posterPath} title={show.title} />
        <button
          type="button"
          className="show-toggle grow"
          aria-expanded={expanded}
          aria-label={`${expanded ? 'Hide' : 'Show'} episodes for ${show.title}`}
          onClick={() => setExpanded((v) => !v)}
        >
          <h3>
            {show.title} <span className="show-caret" aria-hidden="true">{expanded ? '▾' : '▸'}</span>
          </h3>
          <p style={{ margin: 0 }}>{label}</p>
          {episode.airDate !== null && <p className="meta">Aired {episode.airDate}</p>}
        </button>
        <button
          type="button"
          className="check"
          aria-label={`Mark ${show.title} ${code} watched`}
          aria-pressed={watched}
          disabled={watched}
          onClick={() => onMarkWatched(episode.id)}
        >
          &#10003;
        </button>
      </div>
      {expanded && <EpisodeList showId={show.id} />}
    </li>
  )
}
