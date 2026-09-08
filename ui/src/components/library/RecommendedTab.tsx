import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useRecommendedMedia } from '../../hooks/useRelated'
import { dismissRecommendation } from '../../api/related'
import type { RecommendedItem } from '../../api/related'
import { addShow } from '../../api/tv'
import { addMovie } from '../../api/movies'
import { addGameByIgdb } from '../../api/games'
import { listEditions, scanBook, trackBook } from '../../api/books'
import { useRequestSeerrMedia, useSeerrConfigured } from '../../hooks/useSeerr'

interface RecommendedTabProps {
  mediaType: string
  externalId: string
  title: string
  onSelectItem?: (item: { type: string; externalId: string }) => void
}

const TMDB_THUMB = 'https://image.tmdb.org/t/p/w185'

function resolvePosterSrc(path: string): string {
  if (!path) return ''
  if (path.startsWith('http://') || path.startsWith('https://')) return path
  if (path.startsWith('//')) return `https:${path}`
  if (path.startsWith('/images/')) return path
  if (path.startsWith('images/')) return `/${path}`
  if (path.startsWith('/')) return `${TMDB_THUMB}${path}`
  if (path.startsWith('tv/') || path.startsWith('movie/') || path.startsWith('game/') || path.startsWith('book/')) {
    return `/images/${path}`
  }
  return `${TMDB_THUMB}/${path}`
}

function typeBadge(type: string): string {
  switch (type) {
    case 'MOVIE':
      return '🎬 Movie'
    case 'TV':
      return '📺 TV Show'
    case 'BOOK':
      return '📚 Book'
    case 'GAME':
      return '🎮 Game'
    case 'MUSIC':
      return '🎵 Music'
    default:
      return type
  }
}

export default function RecommendedTab({ mediaType, externalId, title, onSelectItem }: RecommendedTabProps) {
  const queryClient = useQueryClient()
  const { data: items, isLoading, isError } = useRecommendedMedia(mediaType, externalId)
  const { data: seerrConfigured } = useSeerrConfigured()
  const requestMedia = useRequestSeerrMedia()
  const [addingKey, setAddingKey] = useState<string | null>(null)
  const [dismissingKey, setDismissingKey] = useState<string | null>(null)
  const [imgFallbacks, setImgFallbacks] = useState<Record<string, string>>({})
  const [imgErrors, setImgErrors] = useState<Record<string, boolean>>({})

  const add = useMutation({
    mutationFn: async (item: RecommendedItem) => {
      setAddingKey(`${item.type}:${item.externalId}`)
      if (item.type === 'MOVIE') {
        await addMovie(Number.parseInt(item.externalId, 10))
      } else if (item.type === 'GAME') {
        await addGameByIgdb(Number.parseInt(item.externalId, 10))
      } else if (item.type === 'BOOK') {
        const editions = await listEditions(item.externalId)
        if (editions.length > 0) {
          const book = await scanBook(editions[0].isbn13)
          await trackBook(book.id, 'PLAN_TO')
        }
      } else {
        await addShow(Number.parseInt(item.externalId, 10))
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['library'] })
      queryClient.invalidateQueries({ queryKey: ['recommendations', mediaType, externalId] })
      setAddingKey(null)
    },
    onError: () => {
      setAddingKey(null)
    },
  })

  const dismiss = useMutation({
    mutationFn: async (item: RecommendedItem) => {
      setDismissingKey(`${item.type}:${item.externalId}`)
      await dismissRecommendation(item.type, item.externalId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['recommendations', mediaType, externalId] })
      setDismissingKey(null)
    },
    onError: () => {
      setDismissingKey(null)
    },
  })

  if (!isLoading && (!items || items.length === 0)) {
    return (
      <div className="related-tab-content">
        <p className="muted" style={{ fontSize: '0.88rem', margin: '0.5rem 0' }}>
          No recommended shows or media found for {title}.
        </p>
      </div>
    )
  }

  const isTV = mediaType.toUpperCase() === 'TV'
  const mediaNoun = isTV ? 'shows' : 'media'

  return (
    <div className="related-tab-content">
      <div className="related-header">
        <p className="muted" style={{ fontSize: '0.85rem', margin: 0 }}>
          Recommended {mediaNoun} heavily aligned with {title}. Click any cover to add it to your library.
        </p>
      </div>

      {isLoading && <p className="muted">Finding heavily aligned recommendations…</p>}
      {isError && <p className="muted">Could not load recommendations.</p>}

      {items && items.length > 0 && (
        <div className="related-grid">
          {items.slice(0, 6).map((item) => {
            const key = `${item.type}:${item.externalId}`
            const posterSrc = imgFallbacks[key] || resolvePosterSrc(item.artworkPath)
            const hasImgError = imgErrors[key]
            const isAdding = addingKey === key
            const isDismissing = dismissingKey === key

            const handleCoverClick = (e: React.MouseEvent | React.KeyboardEvent) => {
              e.stopPropagation()
              if (!item.isTracked && !isAdding) {
                add.mutate(item)
              } else if (item.isTracked && onSelectItem) {
                onSelectItem({ type: item.type, externalId: item.externalId })
              }
            }

            return (
              <div key={key} className="related-card" style={{ opacity: isDismissing ? 0.4 : 1 }}>
                {/* Cover art: Clicking adds it to tracking when unowned, or navigates when tracked */}
                <div
                  className="poster-container"
                  onClick={handleCoverClick}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      handleCoverClick(e)
                    }
                  }}
                  title={
                    item.isTracked
                      ? `In library: View ${item.title}`
                      : `Click to track ${item.title}`
                  }
                  style={{
                    position: 'relative',
                    cursor: 'pointer',
                    flexShrink: 0,
                    borderRadius: '4px',
                    overflow: 'hidden',
                    width: 68,
                    height: 102,
                  }}
                >
                  {posterSrc && !hasImgError ? (
                    <img
                      src={posterSrc}
                      alt={`Cover for ${item.title}`}
                      width={68}
                      height={102}
                      className="poster"
                      style={{
                        objectFit: 'cover',
                        display: 'block',
                        width: '100%',
                        height: '100%',
                        borderRadius: '4px',
                        transition: 'transform 0.15s ease',
                      }}
                      onError={() => {
                        if (posterSrc.startsWith('/images/') && item.artworkPath) {
                          const cleanPath = item.artworkPath.startsWith('/') ? item.artworkPath : `/${item.artworkPath}`
                          const tmdbUrl = `${TMDB_THUMB}${cleanPath}`
                          if (posterSrc !== tmdbUrl) {
                            setImgFallbacks((prev) => ({ ...prev, [key]: tmdbUrl }))
                            return
                          }
                        }
                        setImgErrors((prev) => ({ ...prev, [key]: true }))
                      }}
                    />
                  ) : (
                    <div
                      role="img"
                      aria-label={`No poster for ${item.title}`}
                      className="poster placeholder"
                      style={{
                        width: '100%',
                        height: '100%',
                        fontSize: '1.2rem',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        borderRadius: '4px',
                      }}
                    >
                      {item.title.charAt(0).toUpperCase()}
                    </div>
                  )}

                  {!item.isTracked && (
                    <div
                      style={{
                        position: 'absolute',
                        inset: 0,
                        background: 'rgba(0, 0, 0, 0.65)',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: '#fff',
                        fontSize: '0.75rem',
                        fontWeight: 650,
                        textAlign: 'center',
                        padding: '0.2rem',
                        opacity: isAdding ? 1 : 0,
                        transition: 'opacity 0.15s ease',
                      }}
                      className="cover-add-overlay"
                    >
                      {isAdding ? 'Adding…' : '+ Track'}
                    </div>
                  )}

                  {item.isTracked && (
                    <div
                      style={{
                        position: 'absolute',
                        bottom: 0,
                        left: 0,
                        right: 0,
                        background: 'rgba(0, 0, 0, 0.75)',
                        color: 'var(--confirm, #4ade80)',
                        fontSize: '0.65rem',
                        fontWeight: 600,
                        textAlign: 'center',
                        padding: '0.15rem 0.2rem',
                      }}
                    >
                      ✓ In Library
                    </div>
                  )}
                </div>

                <div className="related-card-info">
                  <div className="related-badges">
                    <span className="badge">{typeBadge(item.type)}</span>
                  </div>

                  <h4
                    className="related-card-title"
                    onClick={() => {
                      if (item.isTracked && onSelectItem) {
                        onSelectItem({ type: item.type, externalId: item.externalId })
                      }
                    }}
                    style={{
                      cursor: item.isTracked ? 'pointer' : 'default',
                    }}
                  >
                    {item.title}
                  </h4>

                  {item.year && item.year > 0 ? (
                    <small className="muted">{item.year}</small>
                  ) : null}

                  {item.overview && (
                    <p className="meta search-overview" style={{ fontSize: '0.78rem' }}>
                      {item.overview}
                    </p>
                  )}

                  {!item.isTracked && (
                    <div style={{ display: 'flex', gap: '0.35rem', marginTop: '0.4rem', flexWrap: 'wrap', alignItems: 'center' }}>
                      <button
                        type="button"
                        className="btn-ghost"
                        style={{
                          fontSize: '0.72rem',
                          padding: '0.15rem 0.45rem',
                          color: 'var(--accent)',
                          borderColor: 'color-mix(in srgb, var(--accent) 35%, var(--border))',
                          fontWeight: 600,
                        }}
                        disabled={isDismissing}
                        onClick={(e) => {
                          e.stopPropagation()
                          dismiss.mutate(item)
                        }}
                        title="Dismiss recommendation"
                      >
                        ✕ Not Interested
                      </button>

                      {seerrConfigured && (item.type === 'TV' || item.type === 'MOVIE') && (
                        <button
                          type="button"
                          className="btn-ghost"
                          style={{ fontSize: '0.72rem', padding: '0.15rem 0.45rem' }}
                          disabled={requestMedia.isPending}
                          onClick={(e) => {
                            e.stopPropagation()
                            const tmdbIdNum = Number.parseInt(item.externalId, 10)
                            if (!Number.isNaN(tmdbIdNum)) {
                              requestMedia.mutate({
                                tmdbId: tmdbIdNum,
                                type: item.type === 'TV' ? 'tv' : 'movie',
                              })
                            }
                          }}
                        >
                          Request on Seerr
                        </button>
                      )}
                    </div>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
