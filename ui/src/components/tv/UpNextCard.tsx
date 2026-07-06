import type { UpNextCard as UpNextCardData } from '../../hooks/useUpNext'
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
 * one-tap watch checkmark. While the mutation is in flight the checkmark shows
 * the optimistic "watched" state (filled); on error the hook rolls it back.
 */
export default function UpNextCard({ entry, onMarkWatched }: UpNextCardProps) {
  const { show, episode } = entry
  const watched = entry.optimisticWatched === true
  const code = episodeCode(episode.season, episode.number)
  const label = episode.title === '' ? code : `${code} · ${episode.title}`

  return (
    <li className={watched ? 'card card-row is-dim' : 'card card-row'}>
      <Poster posterPath={show.posterPath} title={show.title} />
      <div className="grow">
        <h3>{show.title}</h3>
        <p style={{ margin: 0 }}>{label}</p>
        {episode.airDate !== null && <p className="meta">Aired {episode.airDate}</p>}
      </div>
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
    </li>
  )
}
