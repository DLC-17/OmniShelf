import { useRef, useState } from 'react'
import { ApiError } from '../../api/client'
import { BOOK_STATUSES, GAME_STATUSES, MOVIE_STATUSES, TV_STATUSES } from '../../api/library'
import type { ItemStatus, LibraryItem } from '../../api/library'
import { useRefreshArtwork, useUploadArtwork } from '../../hooks/useArtwork'
import { useDeleteItem, useUpdateItem } from '../../hooks/useLibrary'
import EpisodeList from '../tv/EpisodeList'
import Poster from '../tv/Poster'
import RatingStars from './RatingStars'

interface LibraryDetailProps {
  item: LibraryItem
  onClose: () => void
}

/**
 * Expanded detail for one library item, shown in a modal when a cover is
 * clicked. Books surface their cover, author, length and summary; every item
 * offers a self-rating, an inline status change, book page progress, and a
 * confirm-gated delete.
 */
export default function LibraryDetail({ item, onClose }: LibraryDetailProps) {
  const update = useUpdateItem()
  const remove = useDeleteItem()
  const refreshArt = useRefreshArtwork()
  const uploadArt = useUploadArtwork()
  const fileInput = useRef<HTMLInputElement>(null)
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [progressDraft, setProgressDraft] = useState(String(item.progress))
  // Locally-overridden cover src so a refresh/upload shows immediately. A
  // cache-busting query param forces the browser to re-fetch the same path.
  const [artwork, setArtwork] = useState(item.artworkPath)
  const artBusy = refreshArt.isPending || uploadArt.isPending

  const isBook = item.type === 'BOOK'
  const isGame = item.type === 'GAME'
  const isTV = item.type === 'TV'
  const isMovie = item.type === 'MOVIE'
  const statuses = isBook
    ? BOOK_STATUSES
    : isGame
      ? GAME_STATUSES
      : isMovie
        ? MOVIE_STATUSES
        : TV_STATUSES

  const runUpdate = (patch: { status?: ItemStatus; progress?: number; rating?: number }) => {
    setError(null)
    update.mutate(
      { id: item.id, patch },
      { onError: (err) => setError(err instanceof ApiError ? err.message : 'Update failed. Try again.') },
    )
  }

  const commitProgress = () => {
    const parsed = Number.parseInt(progressDraft, 10)
    if (Number.isNaN(parsed) || parsed < 0) {
      setError('Progress must be a non-negative page number.')
      setProgressDraft(String(item.progress))
      return
    }
    if (parsed !== item.progress) runUpdate({ progress: parsed })
  }

  const bust = (path: string) => (path === '' ? '' : `${path}?v=${Date.now()}`)

  const handleRefreshArt = () => {
    setError(null)
    refreshArt.mutate(item.id, {
      onSuccess: (res) => setArtwork(bust(res.artworkPath)),
      onError: (err) =>
        setError(
          err instanceof ApiError
            ? err.message
            : 'Could not refresh the cover. Try again.',
        ),
    })
  }

  const handleUploadArt = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = '' // allow re-selecting the same file later
    if (!file) return
    setError(null)
    uploadArt.mutate(
      { itemId: item.id, file },
      {
        onSuccess: (res) => setArtwork(bust(res.artworkPath)),
        onError: (err) =>
          setError(err instanceof ApiError ? err.message : 'Could not upload the cover. Try again.'),
      },
    )
  }

  const handleDelete = () => {
    setError(null)
    remove.mutate(item.id, {
      onSuccess: onClose,
      onError: (err) => {
        setError(err instanceof ApiError ? err.message : 'Delete failed. Try again.')
        setConfirming(false)
      },
    })
  }

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-label={`${item.title} details`}
        onClick={(e) => e.stopPropagation()}
      >
        <button type="button" className="btn-ghost modal-close" aria-label="Close" onClick={onClose}>
          ✕
        </button>

        <div className="detail">
          <div className="cover-col">
            <Poster posterPath={artwork} title={item.title} width={132} height={198} />
            <div className="cover-actions">
              <button
                type="button"
                className="btn-ghost"
                onClick={handleRefreshArt}
                disabled={artBusy}
              >
                {refreshArt.isPending ? 'Refreshing…' : 'Refresh cover'}
              </button>
              <button
                type="button"
                className="btn-ghost"
                onClick={() => fileInput.current?.click()}
                disabled={artBusy}
              >
                {uploadArt.isPending ? 'Uploading…' : 'Upload cover'}
              </button>
              <input
                ref={fileInput}
                type="file"
                accept="image/*"
                aria-label={`Upload a cover image for ${item.title}`}
                hidden
                onChange={handleUploadArt}
              />
            </div>
          </div>
          <div className="grow">
            <h2>{item.title}</h2>
            {isBook && item.authors !== '' && <p className="muted" style={{ margin: 0 }}>{item.authors}</p>}
            {isBook && item.pageCount > 0 && <p className="meta">{item.pageCount} pages</p>}
            {isGame && item.platform !== '' && <p className="muted" style={{ margin: 0 }}>{item.platform}</p>}

            <div className="detail-rating">
              <span className="muted">Your rating</span>
              <RatingStars
                value={item.rating}
                busy={update.isPending}
                onRate={(rating) => runUpdate({ rating })}
              />
            </div>

            <label className="field" style={{ marginTop: '0.5rem' }}>
              <span>Status</span>
              <select
                aria-label={`Status for ${item.title}`}
                value={item.status}
                disabled={update.isPending}
                onChange={(e) => runUpdate({ status: e.target.value as ItemStatus })}
              >
                {statuses.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </label>

            {isBook && (
              <label className="field" style={{ marginTop: '0.5rem' }}>
                <span>Page</span>
                <input
                  type="number"
                  min={0}
                  aria-label={`Page for ${item.title}`}
                  value={progressDraft}
                  disabled={update.isPending}
                  onChange={(e) => setProgressDraft(e.target.value)}
                  onBlur={commitProgress}
                  style={{ width: '6rem' }}
                />
              </label>
            )}
          </div>
        </div>

        {(isBook || isGame || isMovie) && item.description !== '' && (
          <div className="detail-summary">
            <h3>Summary</h3>
            <p>{item.description}</p>
          </div>
        )}
        {(isBook || isGame || isMovie) && item.description === '' && (
          <p className="muted detail-summary">No summary available.</p>
        )}

        {item.tags.length > 0 && (
          <div className="detail-summary">
            <h3>Tags</h3>
            <div className="tag-list">
              {item.tags.map((tag) => (
                <span key={tag} className="badge">
                  {tag}
                </span>
              ))}
            </div>
          </div>
        )}

        {isTV && item.showId > 0 && (
          <div className="detail-summary">
            <h3>Episodes</h3>
            <EpisodeList showId={item.showId} />
          </div>
        )}

        {error !== null && (
          <p role="alert" className="alert">
            {error}
          </p>
        )}

        <div className="detail-actions">
          {confirming ? (
            <span className="cluster">
              <span className="muted">Remove from library?</span>
              <button type="button" className="btn-danger" onClick={handleDelete} disabled={remove.isPending}>
                Confirm
              </button>
              <button type="button" className="btn-ghost" onClick={() => setConfirming(false)} disabled={remove.isPending}>
                Cancel
              </button>
            </span>
          ) : (
            <button
              type="button"
              className="btn-danger"
              aria-label={`Delete ${item.title}`}
              onClick={() => setConfirming(true)}
            >
              Delete
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
