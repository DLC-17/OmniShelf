import { useState } from 'react'
import { ApiError } from '../../api/client'
import { trackAlbum } from '../../api/music'
import type { Album, MusicStatus } from '../../api/music'

interface AlbumConfirmCardProps {
  album: Album
  /** Reset back to the scanner/manual entry to add another album. */
  onDone: () => void
}

const STATUS_LABELS: Record<MusicStatus, string> = {
  LISTENING: 'Listening',
  PLAN_TO: 'Plan to listen',
  COMPLETED: 'Listened',
  STOPPED: 'Set aside',
}

/**
 * Confirm card for a scanned album: cover (when known), title, artist, year and
 * a status choice, then POST /api/music/track. A 409 already_tracked is
 * reported as an informational message rather than a hard error. Ownership
 * (Vinyl/CD) is set later from the library detail.
 */
export default function AlbumConfirmCard({ album, onDone }: AlbumConfirmCardProps) {
  const [status, setStatus] = useState<MusicStatus>('LISTENING')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [tracked, setTracked] = useState(false)

  const handleTrack = async () => {
    setError(null)
    setNotice(null)
    setSubmitting(true)
    try {
      await trackAlbum(album.id, status)
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

  const coverSrc = album.coverPath !== '' ? `/images/${album.coverPath}` : null

  return (
    <section aria-label="Confirm album" className="card" style={{ maxWidth: '28rem', margin: '0 auto' }}>
      <div className="card-row" style={{ alignItems: 'flex-start' }}>
        {coverSrc !== null ? (
          <img src={coverSrc} alt={`Cover of ${album.title}`} width={96} height={96} className="poster" />
        ) : (
          <div aria-hidden="true" className="poster placeholder" style={{ width: 96, height: 96, fontSize: '0.75rem' }}>
            No cover
          </div>
        )}
        <div className="grow">
          <h2>{album.title}</h2>
          {album.artist !== '' && <p style={{ margin: 0 }}>{album.artist}</p>}
          {album.year > 0 && <p className="meta" style={{ marginTop: '0.25rem' }}>{album.year}</p>}
          <p className="meta" style={{ marginTop: '0.25rem' }}>Barcode {album.barcode}</p>
        </div>
      </div>

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
            <select aria-label="Status" value={status} onChange={(e) => setStatus(e.target.value as MusicStatus)}>
              {(Object.keys(STATUS_LABELS) as MusicStatus[]).map((value) => (
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
