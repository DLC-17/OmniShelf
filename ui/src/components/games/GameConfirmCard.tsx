import { useState } from 'react'
import { ApiError } from '../../api/client'
import { trackGame } from '../../api/games'
import type { Game, GameStatus } from '../../api/games'

interface GameConfirmCardProps {
  game: Game
  /** Reset back to the scanner/manual entry to add another game. */
  onDone: () => void
}

const STATUS_LABELS: Record<GameStatus, string> = {
  PLAYING: 'Playing',
  PLAN_TO: 'Plan to play',
  COMPLETED: 'Completed',
  STOPPED: 'Stopped playing',
}

/**
 * Confirm card for a scanned game: cover (when known), title, platform and a
 * status choice, then POST /api/games/track. A 409 already_tracked is reported
 * as an informational message rather than a hard error.
 */
export default function GameConfirmCard({ game, onDone }: GameConfirmCardProps) {
  const [status, setStatus] = useState<GameStatus>('PLAYING')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [tracked, setTracked] = useState(false)

  const handleTrack = async () => {
    setError(null)
    setNotice(null)
    setSubmitting(true)
    try {
      await trackGame(game.id, status)
      setTracked(true)
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setNotice(err.message)
        setTracked(true)
      } else if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Something went wrong. Please try again.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  // ScanDex does not supply cover art, so CoverPath is usually empty.
  const coverSrc = game.coverPath !== '' ? `/images/${game.coverPath}` : null

  return (
    <section aria-label="Confirm game" className="card" style={{ maxWidth: '28rem', margin: '0 auto' }}>
      <div className="card-row" style={{ alignItems: 'flex-start' }}>
        {coverSrc !== null ? (
          <img src={coverSrc} alt={`Cover of ${game.title}`} width={96} height={144} className="poster" />
        ) : (
          <div aria-hidden="true" className="poster placeholder" style={{ width: 96, height: 144, fontSize: '0.75rem' }}>
            No cover
          </div>
        )}
        <div className="grow">
          <h2>{game.title}</h2>
          {game.platform !== '' && <p style={{ margin: 0 }}>{game.platform}</p>}
          <p className="meta" style={{ marginTop: '0.25rem' }}>Barcode {game.barcode}</p>
        </div>
      </div>

      {game.description !== '' && (
        <p className="muted" style={{ marginTop: '0.75rem' }}>
          {game.description}
        </p>
      )}

      {tracked ? (
        <div className="stack" style={{ marginTop: '1rem' }}>
          <p role="status" className="notice">
            {notice ?? `Added to your shelf as “${STATUS_LABELS[status]}”.`}
          </p>
          <div>
            <button type="button" className="btn-ghost" onClick={onDone}>
              Scan another
            </button>
          </div>
        </div>
      ) : (
        <div className="stack" style={{ marginTop: '1rem' }}>
          <label className="field">
            <span>Status</span>
            <select aria-label="Status" value={status} onChange={(e) => setStatus(e.target.value as GameStatus)}>
              {(Object.keys(STATUS_LABELS) as GameStatus[]).map((value) => (
                <option key={value} value={value}>
                  {STATUS_LABELS[value]}
                </option>
              ))}
            </select>
          </label>
          {error !== null && (
            <p role="alert" className="alert">
              {error}
            </p>
          )}
          <div className="cluster">
            <button type="button" className="btn-confirm" onClick={handleTrack} disabled={submitting}>
              {submitting ? 'Adding…' : 'Add to shelf'}
            </button>
            <button type="button" className="btn-ghost" onClick={onDone} disabled={submitting}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </section>
  )
}
