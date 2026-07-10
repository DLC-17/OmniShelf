import { useState } from 'react'
import { ApiError } from '../../api/client'
import { BOOK_STATUSES, GAME_STATUSES, MOVIE_STATUSES, MUSIC_STATUSES, TV_STATUSES } from '../../api/library'
import type { ItemStatus, LibraryItem } from '../../api/library'
import { useDeleteItem, useUpdateItem } from '../../hooks/useLibrary'
import { MUSIC_OWNERSHIP, OwnershipSelect } from '../common/OwnershipSelect'
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
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [progressDraft, setProgressDraft] = useState(String(item.progress))

  const isBook = item.type === 'BOOK'
  const isGame = item.type === 'GAME'
  const isTV = item.type === 'TV'
  const isMovie = item.type === 'MOVIE'
  const isMusic = item.type === 'MUSIC'
  const statuses = isBook
    ? BOOK_STATUSES
    : isGame
      ? GAME_STATUSES
      : isMovie
        ? MOVIE_STATUSES
        : isMusic
          ? MUSIC_STATUSES
          : TV_STATUSES

  const runUpdate = (patch: {
    status?: ItemStatus
    progress?: number
    rating?: number
    ownership?: string[]
  }) => {
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
          <Poster posterPath={item.artworkPath} title={item.title} width={132} height={198} />
          <div className="grow">
            <h2>{item.title}</h2>
            {isBook && item.authors !== '' && <p className="muted" style={{ margin: 0 }}>{item.authors}</p>}
            {isBook && item.pageCount > 0 && <p className="meta">{item.pageCount} pages</p>}
            {isGame && item.platform !== '' && <p className="muted" style={{ margin: 0 }}>{item.platform}</p>}
            {isMusic && item.artist !== '' && <p className="muted" style={{ margin: 0 }}>{item.artist}</p>}
            {isMusic && item.year > 0 && <p className="meta">{item.year}</p>}

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

            {isMusic && (
              <div className="field" style={{ marginTop: '0.5rem' }}>
                <OwnershipSelect
                  options={MUSIC_OWNERSHIP}
                  value={item.ownership}
                  disabled={update.isPending}
                  label={`Ownership for ${item.title}`}
                  onChange={(ownership) => runUpdate({ ownership })}
                />
              </div>
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
