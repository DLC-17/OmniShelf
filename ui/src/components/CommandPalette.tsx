import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { searchShows } from '../api/tv'
import { searchMovies } from '../api/movies'
import { searchBooks } from '../api/books'
import { searchGames } from '../api/games'
import { searchMusic } from '../api/music'
import { useTheme } from '../theme/ThemeContext'

export type PaletteCategory =
  | 'all'
  | 'actions'
  | 'tv'
  | 'movies'
  | 'books'
  | 'games'
  | 'music'
  | 'cards'

export interface PaletteItem {
  id: string
  title: string
  subtitle?: string
  category: 'Actions' | 'TV' | 'Movies' | 'Books' | 'Games' | 'Music' | 'Cards'
  icon?: string
  image?: string
  action: () => void
}

interface CommandPaletteProps {
  isOpen?: boolean
  onClose?: () => void
}

const CATEGORY_TABS: { id: PaletteCategory; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'actions', label: 'Actions' },
  { id: 'tv', label: 'TV Shows' },
  { id: 'movies', label: 'Movies' },
  { id: 'books', label: 'Books' },
  { id: 'games', label: 'Games' },
  { id: 'music', label: 'Music' },
  { id: 'cards', label: 'Cards' },
]

export default function CommandPalette({ isOpen: controlledIsOpen, onClose }: CommandPaletteProps) {
  const [internalOpen, setInternalOpen] = useState(false)
  const isControlled = controlledIsOpen !== undefined
  const isOpen = isControlled ? controlledIsOpen : internalOpen

  const [query, setQuery] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<PaletteCategory>('all')
  const [selectedIndex, setSelectedIndex] = useState(0)
  const [isSearching, setIsSearching] = useState(false)
  const [mediaResults, setMediaResults] = useState<PaletteItem[]>([])

  const inputRef = useRef<HTMLInputElement>(null)
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([])
  const navigate = useNavigate()
  const { setTheme } = useTheme()

  const handleClose = () => {
    if (onClose) {
      onClose()
    } else {
      setInternalOpen(false)
    }
    setQuery('')
    setSelectedIndex(0)
    setMediaResults([])
  }

  // Listen to global Ctrl+K / Cmd+K shortcut
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        if (isOpen) {
          handleClose()
        } else {
          if (onClose && isControlled) {
            // Controlled from outside
          } else {
            setInternalOpen(true)
          }
        }
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, isControlled, onClose])

  // Focus input on open
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [isOpen])

  // Built-in actions and shortcuts
  const baseActions = useMemo<PaletteItem[]>(() => [
    {
      id: 'action-scan',
      title: 'Open Barcode Scanner',
      subtitle: 'Scan ISBNs, game barcodes, vinyl, or trading cards',
      category: 'Actions',
      icon: '📷',
      action: () => {
        handleClose()
        navigate('/scan')
      },
    },
    {
      id: 'action-scan-book',
      title: 'Scan Books (ISBN)',
      subtitle: 'Open book camera or handheld barcode scanner',
      category: 'Books',
      icon: '📖',
      action: () => {
        handleClose()
        navigate('/scan?media=book')
      },
    },
    {
      id: 'action-scan-game',
      title: 'Scan Games (UPC/EAN)',
      subtitle: 'Identify and track physical games',
      category: 'Games',
      icon: '🎮',
      action: () => {
        handleClose()
        navigate('/scan?media=game')
      },
    },
    {
      id: 'action-scan-music',
      title: 'Scan Music / Vinyl Barcode',
      subtitle: 'Identify albums via Discogs barcode scanning',
      category: 'Music',
      icon: '🎵',
      action: () => {
        handleClose()
        navigate('/scan?media=music')
      },
    },
    {
      id: 'action-scan-cards',
      title: 'Scan Trading Cards (AI Vision)',
      subtitle: 'Photograph Pokémon or Yu-Gi-Oh! cards',
      category: 'Cards',
      icon: '🃏',
      action: () => {
        handleClose()
        navigate('/scan?media=card')
      },
    },
    {
      id: 'nav-upnext',
      title: 'Up Next',
      subtitle: 'View your TV watch dashboard and upcoming episodes',
      category: 'TV',
      icon: '📺',
      action: () => {
        handleClose()
        navigate('/')
      },
    },
    {
      id: 'nav-discover',
      title: 'Discover New Media',
      subtitle: 'Browse personalized recommendations for TV, movies, games, and books',
      category: 'Actions',
      icon: '🧭',
      action: () => {
        handleClose()
        navigate('/discover')
      },
    },
    {
      id: 'nav-library-tv',
      title: 'TV Shows Library',
      subtitle: 'Browse your tracked TV shows and episode progress',
      category: 'TV',
      icon: '📺',
      action: () => {
        handleClose()
        navigate('/library?type=TV')
      },
    },
    {
      id: 'nav-library-movies',
      title: 'Movies Library',
      subtitle: 'Browse your movie watchlist and watched films',
      category: 'Movies',
      icon: '🎬',
      action: () => {
        handleClose()
        navigate('/library?type=MOVIE')
      },
    },
    {
      id: 'nav-library-books',
      title: 'Books Library',
      subtitle: 'Browse your reading shelf and page progress',
      category: 'Books',
      icon: '📚',
      action: () => {
        handleClose()
        navigate('/library?type=BOOK')
      },
    },
    {
      id: 'nav-library-games',
      title: 'Games Library',
      subtitle: 'Browse your game backlog and played titles',
      category: 'Games',
      icon: '🎮',
      action: () => {
        handleClose()
        navigate('/library?type=GAME')
      },
    },
    {
      id: 'nav-library-music',
      title: 'Music Library',
      subtitle: 'Browse your album and vinyl collection',
      category: 'Music',
      icon: '🎵',
      action: () => {
        handleClose()
        navigate('/library?type=MUSIC')
      },
    },
    {
      id: 'nav-library-cards',
      title: 'Trading Cards Library',
      subtitle: 'Browse card binder, sets, and market values',
      category: 'Cards',
      icon: '🃏',
      action: () => {
        handleClose()
        navigate('/library?type=CARD')
      },
    },
    {
      id: 'nav-profile',
      title: 'View Profile & Stats',
      subtitle: 'View watch stats, heatmaps, and achievements',
      category: 'Actions',
      icon: '👤',
      action: () => {
        handleClose()
        navigate('/settings')
      },
    },
    {
      id: 'nav-settings',
      title: 'Go to Settings',
      subtitle: 'Customize themes, manage imports, and export library data',
      category: 'Actions',
      icon: '⚙️',
      action: () => {
        handleClose()
        navigate('/settings')
      },
    },
    {
      id: 'nav-import',
      title: 'Import Data',
      subtitle: 'Import history from TV Time or Goodreads',
      category: 'Actions',
      icon: '📥',
      action: () => {
        handleClose()
        navigate('/import')
      },
    },
    {
      id: 'theme-espresso',
      title: 'Theme: Dark Espresso',
      subtitle: 'Switch to warm dark espresso palette',
      category: 'Actions',
      icon: '☕',
      action: () => {
        setTheme('dark-espresso')
        handleClose()
      },
    },
    {
      id: 'theme-oled',
      title: 'Theme: Midnight OLED',
      subtitle: 'Switch to true pitch black OLED theme',
      category: 'Actions',
      icon: '🌑',
      action: () => {
        setTheme('midnight-oled')
        handleClose()
      },
    },
    {
      id: 'theme-cream',
      title: 'Theme: Cream Paper',
      subtitle: 'Switch to light vintage paper aesthetic',
      category: 'Actions',
      icon: '📜',
      action: () => {
        setTheme('cream-paper')
        handleClose()
      },
    },
    {
      id: 'theme-dracula',
      title: 'Theme: Dracula',
      subtitle: 'Classic Dracula — deep purples with neon accents',
      category: 'Actions',
      icon: '🧛',
      action: () => {
        setTheme('dracula')
        handleClose()
      },
    },
    {
      id: 'theme-obsidian',
      title: 'Theme: Obsidian',
      subtitle: 'Volcanic glass — inky blacks with warm amber',
      category: 'Actions',
      icon: '🪨',
      action: () => {
        setTheme('obsidian')
        handleClose()
      },
    },
  ], [navigate, setTheme])

  // Filter actions based on text query
  const filteredActions = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return baseActions
    return baseActions.filter(
      (item) =>
        item.title.toLowerCase().includes(q) ||
        (item.subtitle && item.subtitle.toLowerCase().includes(q)) ||
        item.category.toLowerCase().includes(q),
    )
  }, [baseActions, query])

  // Debounced multi-category live search
  useEffect(() => {
    const trimmed = query.trim()
    if (trimmed.length < 2) {
      setMediaResults([])
      setIsSearching(false)
      return
    }

    setIsSearching(true)
    let cancelled = false

    const timer = setTimeout(async () => {
      const results: PaletteItem[] = []

      try {
        const [tvRes, movieRes, bookRes, gameRes, musicRes] = await Promise.allSettled([
          searchShows(trimmed),
          searchMovies(trimmed),
          searchBooks(trimmed),
          searchGames(trimmed),
          searchMusic(trimmed),
        ])

        if (cancelled) return

        // TV Results
        if (tvRes.status === 'fulfilled' && Array.isArray(tvRes.value)) {
          tvRes.value.slice(0, 4).forEach((show) => {
            results.push({
              id: `tv-${show.id}`,
              title: show.name,
              subtitle: show.firstAirDate ? `TV Series (${show.firstAirDate.slice(0, 4)})` : 'TV Series',
              category: 'TV',
              icon: '📺',
              image: show.posterPath ? `https://image.tmdb.org/t/p/w92${show.posterPath}` : undefined,
              action: () => {
                handleClose()
                navigate(`/library?type=TV`)
              },
            })
          })
        }

        // Movie Results
        if (movieRes.status === 'fulfilled' && Array.isArray(movieRes.value)) {
          movieRes.value.slice(0, 4).forEach((movie) => {
            results.push({
              id: `movie-${movie.id}`,
              title: movie.title,
              subtitle: movie.releaseDate ? `Movie (${movie.releaseDate.slice(0, 4)})` : 'Movie',
              category: 'Movies',
              icon: '🎬',
              image: movie.posterPath ? `https://image.tmdb.org/t/p/w92${movie.posterPath}` : undefined,
              action: () => {
                handleClose()
                navigate(`/library?type=MOVIE`)
              },
            })
          })
        }

        // Book Results
        if (bookRes.status === 'fulfilled' && Array.isArray(bookRes.value)) {
          bookRes.value.slice(0, 4).forEach((book) => {
            results.push({
              id: `book-${book.workKey}`,
              title: book.title,
              subtitle: book.authors ? `Book by ${book.authors}` : 'Book',
              category: 'Books',
              icon: '📚',
              image: book.coverId ? `/api/covers/openlibrary/${book.coverId}` : undefined,
              action: () => {
                handleClose()
                navigate(`/library?type=BOOK`)
              },
            })
          })
        }

        // Game Results
        if (gameRes.status === 'fulfilled' && Array.isArray(gameRes.value)) {
          gameRes.value.slice(0, 4).forEach((game) => {
            results.push({
              id: `game-${game.igdbId}`,
              title: game.name,
              subtitle: game.year ? `Game (${game.year})` : 'Game',
              category: 'Games',
              icon: '🎮',
              image: game.coverImageId ? `/api/covers/igdb/${game.coverImageId}` : undefined,
              action: () => {
                handleClose()
                navigate(`/library?type=GAME`)
              },
            })
          })
        }

        // Music Results
        if (musicRes.status === 'fulfilled' && Array.isArray(musicRes.value)) {
          musicRes.value.slice(0, 4).forEach((album) => {
            results.push({
              id: `music-${album.mbid}`,
              title: album.title,
              subtitle: album.artist ? `Album by ${album.artist}` : 'Album',
              category: 'Music',
              icon: '🎵',
              action: () => {
                handleClose()
                navigate(`/library?type=MUSIC`)
              },
            })
          })
        }

        if (!cancelled) {
          setMediaResults(results)
        }
      } catch {
        // search failures should not crash palette
      } finally {
        if (!cancelled) {
          setIsSearching(false)
        }
      }
    }, 220)

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [query, navigate])

  // Combine and filter items by selectedCategory tab
  const combinedItems = useMemo(() => {
    const all = [...filteredActions, ...mediaResults]

    if (selectedCategory === 'all') return all
    if (selectedCategory === 'actions') return all.filter((i) => i.category === 'Actions')
    if (selectedCategory === 'tv') return all.filter((i) => i.category === 'TV')
    if (selectedCategory === 'movies') return all.filter((i) => i.category === 'Movies')
    if (selectedCategory === 'books') return all.filter((i) => i.category === 'Books')
    if (selectedCategory === 'games') return all.filter((i) => i.category === 'Games')
    if (selectedCategory === 'music') return all.filter((i) => i.category === 'Music')
    if (selectedCategory === 'cards') return all.filter((i) => i.category === 'Cards')
    return all
  }, [filteredActions, mediaResults, selectedCategory])

  // Group items for display
  const groupedItems = useMemo(() => {
    const groups: { title: string; items: PaletteItem[] }[] = []
    const categoryMap = new Map<string, PaletteItem[]>()

    for (const item of combinedItems) {
      const list = categoryMap.get(item.category) ?? []
      list.push(item)
      categoryMap.set(item.category, list)
    }

    categoryMap.forEach((items, category) => {
      groups.push({ title: category, items })
    })

    return groups
  }, [combinedItems])

  // Reset selectedIndex when items change
  useEffect(() => {
    setSelectedIndex(0)
  }, [query, selectedCategory])

  // Scroll active item into view
  useEffect(() => {
    if (itemRefs.current[selectedIndex]) {
      itemRefs.current[selectedIndex]?.scrollIntoView({
        block: 'nearest',
        behavior: 'smooth',
      })
    }
  }, [selectedIndex])

  // Keyboard navigation within the palette
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      handleClose()
      return
    }

    if (combinedItems.length === 0) return

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelectedIndex((prev) => (prev + 1) % combinedItems.length)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelectedIndex((prev) => (prev - 1 + combinedItems.length) % combinedItems.length)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const currentItem = combinedItems[selectedIndex]
      if (currentItem) {
        currentItem.action()
      }
    }
  }

  if (!isOpen) return null

  let globalIndex = 0

  return (
    <div
      className="command-palette-backdrop"
      onClick={(e) => {
        if (e.target === e.currentTarget) handleClose()
      }}
      role="dialog"
      aria-modal="true"
      aria-label="Command Palette"
    >
      <div className="command-palette-modal" onKeyDown={handleKeyDown}>
        <div className="command-palette-header">
          <span className="command-palette-icon" aria-hidden="true">
            🔍
          </span>
          <input
            ref={inputRef}
            type="text"
            className="command-palette-input"
            placeholder="Type a command, search TV, movies, books, games, music..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label="Command search query"
          />
          {query.length > 0 && (
            <button
              type="button"
              className="command-palette-clear"
              onClick={() => {
                setQuery('')
                inputRef.current?.focus()
              }}
              aria-label="Clear query"
            >
              ✕
            </button>
          )}
          <span className="nav-kbd">Esc</span>
        </div>

        <div className="command-palette-tabs" role="tablist" aria-label="Command categories">
          {CATEGORY_TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={selectedCategory === tab.id}
              className={selectedCategory === tab.id ? 'command-palette-tab active' : 'command-palette-tab'}
              onClick={() => setSelectedCategory(tab.id)}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="command-palette-body">
          {isSearching && (
            <div style={{ padding: '0.4rem 1rem', fontSize: '0.78rem', color: 'var(--muted)' }}>
              Searching media databases…
            </div>
          )}

          {combinedItems.length === 0 ? (
            <div className="command-palette-empty">
              No matching actions or media found for &ldquo;{query}&rdquo;
            </div>
          ) : (
            groupedItems.map((group) => (
              <div key={group.title} className="command-palette-group">
                <div className="command-palette-group-title">{group.title}</div>
                <ul className="command-palette-list">
                  {group.items.map((item) => {
                    const currentIndex = globalIndex++
                    const isSelected = currentIndex === selectedIndex
                    return (
                      <li key={item.id}>
                        <button
                          ref={(el) => {
                            itemRefs.current[currentIndex] = el
                          }}
                          type="button"
                          className={`command-palette-item ${isSelected ? 'active' : ''}`}
                          onClick={item.action}
                          onMouseEnter={() => setSelectedIndex(currentIndex)}
                        >
                          {item.image ? (
                            <img
                              src={item.image}
                              alt=""
                              className="command-palette-item-thumb"
                              onError={(e) => {
                                (e.currentTarget as HTMLElement).style.display = 'none'
                              }}
                            />
                          ) : (
                            <span className="command-palette-item-icon" aria-hidden="true">
                              {item.icon ?? '✨'}
                            </span>
                          )}
                          <div className="command-palette-item-content">
                            <span className="command-palette-item-title">{item.title}</span>
                            {item.subtitle && (
                              <span className="command-palette-item-meta">{item.subtitle}</span>
                            )}
                          </div>
                          <span className="command-palette-item-tag">{item.category}</span>
                        </button>
                      </li>
                    )
                  })}
                </ul>
              </div>
            ))
          )}
        </div>

        <div className="command-palette-footer">
          <div className="command-palette-footer-hints">
            <span className="command-palette-footer-hint">
              <kbd className="nav-kbd">↑</kbd> <kbd className="nav-kbd">↓</kbd> Navigate
            </span>
            <span className="command-palette-footer-hint">
              <kbd className="nav-kbd">↵</kbd> Select
            </span>
            <span className="command-palette-footer-hint">
              <kbd className="nav-kbd">Esc</kbd> Close
            </span>
          </div>
          <span>OmniShelf Global Search</span>
        </div>
      </div>
    </div>
  )
}
