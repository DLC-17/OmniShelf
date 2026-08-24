import { useRef, useState } from 'react'
import { ApiError } from '../../api/client'
import {
  BOOK_STATUSES,
  CARD_OWNERSHIP,
  CARD_STATUSES,
  GAME_OWNERSHIP,
  GAME_STATUSES,
  MOVIE_STATUSES,
  MUSIC_OWNERSHIP,
  MUSIC_STATUSES,
  TV_STATUSES,
} from '../../api/library'
import type { ItemStatus, LibraryItem } from '../../api/library'
import { formatUsd } from '../../lib/currency'
import { useRefreshArtwork, useUploadArtwork } from '../../hooks/useArtwork'
import { useDeleteItem, useUpdateItem, useUpdateOwnership } from '../../hooks/useLibrary'
import { useRequestSeerrMedia, useSeerrConfigured, useSeerrStatus } from '../../hooks/useSeerr'
import OwnershipSelect from '../common/OwnershipSelect'
import EpisodeList from '../tv/EpisodeList'
import Poster from '../tv/Poster'
import BookNotes from './BookNotes'
import RatingStars from './RatingStars'
import PriceSparkline from '../cards/PriceSparkline'

interface LibraryDetailProps {
  item: LibraryItem
  onClose: () => void
}

/**
 * Expanded detail for one library item, shown in a modal when a cover is
 * clicked. Media surfaces their cover, metadata and summary; every item
 * offers a self-rating, an inline status change, shelf location tagging,
 * universal journaling with emotional reaction chips, and a confirm-gated delete.
 */
export default function LibraryDetail({ item, onClose }: LibraryDetailProps) {
  const update = useUpdateItem()
  const remove = useDeleteItem()
  const updateOwnership = useUpdateOwnership()
  const refreshArt = useRefreshArtwork()
  const uploadArt = useUploadArtwork()
  const fileInput = useRef<HTMLInputElement>(null)
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [progressDraft, setProgressDraft] = useState(String(item.progress))
  const [locationDraft, setLocationDraft] = useState(item.location ?? '')
  // Locally-held ownership so toggles reflect immediately; reconciled to the
  // server's canonical set on success and rolled back on error.
  const [ownership, setOwnership] = useState<string[]>(item.ownership)
  // Locally-overridden cover src so a refresh/upload shows immediately. A
  // cache-busting query param forces the browser to re-fetch the same path.
  const [artwork, setArtwork] = useState(item.artworkPath)
  const artBusy = refreshArt.isPending || uploadArt.isPending

  const { data: seerrConfigured } = useSeerrConfigured()
  const isTV = item.type === 'TV'
  const isMovie = item.type === 'MOVIE'
  const tmdbId = Number.parseInt(item.externalId, 10)
  const isSeerrEligible = (isTV || isMovie) && seerrConfigured && !Number.isNaN(tmdbId)
  const seerrType = isTV ? 'tv' : 'movie'
  const { data: seerrStatus } = useSeerrStatus(tmdbId, seerrType, !!isSeerrEligible)
  const requestMedia = useRequestSeerrMedia()

  const handleRequestMedia = () => {
    requestMedia.mutate({ tmdbId, type: seerrType })
  }

  const isBook = item.type === 'BOOK'
  const isGame = item.type === 'GAME'
  const isMusic = item.type === 'MUSIC'
  const isCard = item.type === 'CARD'
  const statuses = isBook
    ? BOOK_STATUSES
    : isGame
      ? GAME_STATUSES
      : isMovie
        ? MOVIE_STATUSES
        : isMusic
          ? MUSIC_STATUSES
          : isCard
            ? CARD_STATUSES
            : TV_STATUSES

  const runUpdate = (patch: {
    status?: ItemStatus
    progress?: number
    rating?: number
    ownership?: string[]
    location?: string
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

  const commitLocation = () => {
    const trimmed = locationDraft.trim()
    if (trimmed !== (item.location ?? '')) {
      runUpdate({ location: trimmed })
    }
  }

  const handleOwnership = (next: string[]) => {
    setError(null)
    const prev = ownership
    setOwnership(next) // optimistic
    updateOwnership.mutate(
      { id: item.id, formats: next },
      {
        onSuccess: (canonical) => setOwnership(canonical),
        onError: (err) => {
          setOwnership(prev)
          setError(err instanceof ApiError ? err.message : 'Could not update ownership. Try again.')
        },
      },
    )
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
            {isMusic && item.artist !== '' && <p className="muted" style={{ margin: 0 }}>{item.artist}</p>}
            {isMusic && item.year > 0 && <p className="meta">{item.year}</p>}
            {isCard && item.platform !== '' && (
              <p className="muted" style={{ margin: 0 }}>
                {item.platform}
                {item.setCode !== '' && ` · ${item.setCode}`}
              </p>
            )}
            {isCard && item.artist !== '' && <p className="meta">Illus. {item.artist}</p>}
            {isCard && item.price > 0 && <p className="meta">Market price {formatUsd(item.price)}</p>}
            {isCard && (
              <div className="field" style={{ marginTop: '0.5rem' }}>
                <OwnershipSelect
                  options={CARD_OWNERSHIP}
                  selected={ownership}
                  disabled={updateOwnership.isPending}
                  label={`Finish for ${item.title}`}
                  onChange={handleOwnership}
                />
              </div>
            )}

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
                {statuses.map((s) => {
                  let label: string = s
                  if (s === 'PLAN_TO') label = 'Not started'
                  else if (s === 'WATCHING') label = 'Watching'
                  else if (s === 'READING') label = 'Reading'
                  else if (s === 'PLAYING') label = 'Playing'
                  else if (s === 'LISTENING') label = 'Listening'
                  else if (s === 'COMPLETED') label = 'Completed'
                  else if (s === 'STOPPED') {
                    if (item.type === 'TV' || item.type === 'MOVIE') label = 'Stopped watching'
                    else if (item.type === 'BOOK') label = 'Stopped reading'
                    else if (item.type === 'GAME') label = 'Stopped playing'
                    else if (item.type === 'MUSIC') label = 'Set aside'
                    else label = 'Stopped'
                  }
                  return (
                    <option key={s} value={s} disabled={isTV && s === 'COMPLETED'}>
                      {label}
                    </option>
                  )
                })}
              </select>
            </label>

            {/* Shelf Location Tag Input */}
            <label className="field" style={{ marginTop: '0.5rem' }}>
              <span>Shelf Location</span>
              <input
                type="text"
                placeholder="e.g. Living Room Shelf A"
                aria-label={`Shelf Location for ${item.title}`}
                value={locationDraft}
                disabled={update.isPending}
                onChange={(e) => setLocationDraft(e.target.value)}
                onBlur={commitLocation}
                style={{ flex: 1, minWidth: '10rem' }}
              />
            </label>

            {isSeerrEligible && (
              <div className="field" style={{ marginTop: '0.5rem' }}>
                <span>Seerr</span>
                {seerrStatus?.available ? (
                  <span className="badge">Available</span>
                ) : seerrStatus?.requested ? (
                  <span className="badge">Requested</span>
                ) : (
                  <button
                    type="button"
                    className="btn-ghost"
                    onClick={handleRequestMedia}
                    disabled={requestMedia.isPending || !seerrStatus}
                    style={{ padding: '0.25rem 0.5rem', fontSize: '0.85rem' }}
                  >
                    {requestMedia.isPending ? 'Requesting…' : 'Request Media'}
                  </button>
                )}
              </div>
            )}

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
                  selected={ownership}
                  disabled={updateOwnership.isPending}
                  label={`Ownership for ${item.title}`}
                  onChange={handleOwnership}
                />
              </div>
            )}
          </div>
        </div>

        {isCard && item.price > 0 && (
          <div className="detail-summary">
            <h3>Price History</h3>
            <PriceSparkline currentPrice={item.price} width={450} height={90} />
          </div>
        )}

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

        {isGame && (
          <div className="detail-summary">
            <h3>Ownership</h3>
            <OwnershipSelect
              options={GAME_OWNERSHIP}
              selected={ownership}
              disabled={updateOwnership.isPending}
              onChange={handleOwnership}
              label={`Ownership for ${item.title}`}
            />
          </div>
        )}

        {isTV && item.showId > 0 && (
          <div className="detail-summary">
            <h3>Episodes</h3>
            <EpisodeList showId={item.showId} />
          </div>
        )}

        {/* Universal Cross-Media Journal & Sentiment Reactions */}
        <BookNotes itemId={item.id} />

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
