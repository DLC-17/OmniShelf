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
    <li
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '1rem',
        padding: '0.75rem',
        border: '1px solid #ccc',
        borderRadius: 8,
        opacity: watched ? 0.6 : 1,
      }}
    >
      <Poster posterPath={show.posterPath} title={show.title} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <h3 style={{ margin: '0 0 0.25rem' }}>{show.title}</h3>
        <p style={{ margin: 0 }}>{label}</p>
        {episode.airDate !== null && (
          <p style={{ margin: 0, color: '#666', fontSize: '0.85rem' }}>Aired {episode.airDate}</p>
        )}
      </div>
      <button
        type="button"
        aria-label={`Mark ${show.title} ${code} watched`}
        aria-pressed={watched}
        disabled={watched}
        onClick={() => onMarkWatched(episode.id)}
        style={{
          width: '2.75rem',
          height: '2.75rem',
          borderRadius: '50%',
          border: '2px solid #2e7d32',
          background: watched ? '#2e7d32' : 'transparent',
          color: watched ? '#fff' : '#2e7d32',
          fontSize: '1.25rem',
          cursor: watched ? 'default' : 'pointer',
          flexShrink: 0,
        }}
      >
        &#10003;
      </button>
    </li>
  )
}
