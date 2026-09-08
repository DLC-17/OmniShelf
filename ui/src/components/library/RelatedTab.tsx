import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useRelatedMedia } from '../../hooks/useRelated'
import type { RelatedItem } from '../../api/related'
import { addShow } from '../../api/tv'
import { addMovie } from '../../api/movies'
import { addGameByIgdb } from '../../api/games'
import { listEditions, scanBook, trackBook } from '../../api/books'

interface RelatedTabProps {
  mediaType: string
  externalId: string
  title: string
  onSelectItem?: (item: RelatedItem) => void
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

function relationLabel(rel: string): string {
  switch (rel) {
    case 'sequel':
      return 'Sequel'
    case 'prequel':
      return 'Prequel'
    case 'adaptation':
      return 'Adaptation'
    case 'same_series':
      return 'Same Series'
    case 'same_universe':
      return 'Shared Universe'
    default:
      return 'Related'
  }
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

export default function RelatedTab({ mediaType, externalId, title, onSelectItem }: RelatedTabProps) {
  const queryClient = useQueryClient()
  const { data: items, isLoading, isError } = useRelatedMedia(mediaType, externalId)
  const [addingKey, setAddingKey] = useState<string | null>(null)
  const [imgFallbacks, setImgFallbacks] = useState<Record<string, string>>({})
  const [imgErrors, setImgErrors] = useState<Record<string, boolean>>({})

  const add = useMutation({
    mutationFn: async (item: RelatedItem) => {
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
      queryClient.invalidateQueries({ queryKey: ['related', mediaType, externalId] })
      setAddingKey(null)
    },
    onError: () => {
      setAddingKey(null)
    },
  })

  if (!isLoading && (!items || items.length === 0)) {
    return null
  }

  return (
    <div className="related-tab-content">
      <div className="related-header">
        <p className="muted" style={{ fontSize: '0.85rem', margin: 0 }}>
          Connected franchise material for {title}. Click any cover to add it to your library.
        </p>
      </div>

      {isLoading && <p className="muted">Discovering connected media…</p>}
      {isError && <p className="muted">Could not load related entries.</p>}

      {items && items.length > 0 && (
        <div className="related-grid">
          {items.map((item) => {
            const key = `${item.type}:${item.externalId}`
            const posterSrc = imgFallbacks[key] || resolvePosterSrc(item.artworkPath)
            const hasImgError = imgErrors[key]
            const isAdding = addingKey === key

            const handleCoverClick = (e: React.MouseEvent | React.KeyboardEvent) => {
              e.stopPropagation()
              if (!item.isTracked && !isAdding) {
                add.mutate(item)
              } else if (item.isTracked && onSelectItem) {
                onSelectItem(item)
              }
            }

            return (
              <div key={key} className="related-card">
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
                    <span className="badge badge-relation">{relationLabel(item.relationType)}</span>
                    <span className="badge">{typeBadge(item.type)}</span>
                  </div>
                  <h4
                    className="related-card-title"
                    onClick={() => {
                      if (item.isTracked && onSelectItem) {
                        onSelectItem(item)
                      }
                    }}
                    style={{
                      cursor: item.isTracked ? 'pointer' : 'default',
                    }}
                  >
                    {item.title}
                  </h4>
                  {item.year && item.year > 0 && (
                    <small className="muted">{item.year}</small>
                  )}
                  {item.overview && (
                    <p className="meta search-overview" style={{ fontSize: '0.78rem' }}>
                      {item.overview}
                    </p>
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
