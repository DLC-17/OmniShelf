import { useMemo, useState } from 'react'
import { ApiError } from '../api/client'
import type { LibraryItem } from '../api/library'
import { useLibrary } from '../hooks/useLibrary'
import LibraryDetail from '../components/library/LibraryDetail'
import Poster from '../components/tv/Poster'
import { formatUsd } from '../lib/currency'

/** The TCG a card belongs to, derived from its source-prefixed external id. */
function cardGameLabel(item: LibraryItem): string {
  if (item.externalId.startsWith('ygo:')) return 'Yu-Gi-Oh!'
  if (item.externalId.startsWith('ptcg:')) return 'Pokémon'
  return 'Other'
}

/** Groups cards by TCG, then within each game by set. */
function groupByGameAndSet(
  items: LibraryItem[],
): { game: string; sets: { set: string; cards: LibraryItem[] }[] }[] {
  const byGame = new Map<string, Map<string, LibraryItem[]>>()
  for (const item of items) {
    const game = cardGameLabel(item)
    const set = item.platform.trim() === '' ? 'Unknown set' : item.platform
    const sets = byGame.get(game) ?? new Map<string, LibraryItem[]>()
    byGame.set(game, sets)
    const bucket = sets.get(set)
    if (bucket) bucket.push(item)
    else sets.set(set, [item])
  }
  const numberOf = (i: LibraryItem) => parseInt(i.setCode, 10)
  const alpha = (a: string, b: string) => a.localeCompare(b, undefined, { sensitivity: 'base' })
  return [...byGame.entries()]
    .sort(([a], [b]) => alpha(a, b))
    .map(([game, sets]) => ({
      game,
      sets: [...sets.entries()]
        .sort(([a], [b]) => alpha(a, b))
        .map(([set, cards]) => ({
          set,
          cards: [...cards].sort((a, b) => {
            const an = numberOf(a)
            const bn = numberOf(b)
            if (!Number.isNaN(an) && !Number.isNaN(bn) && an !== bn) return an - bn
            return alpha(a.title, b.title)
          }),
        })),
    }))
}

export default function Cards() {
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState('')
  const [cardFilter, setCardFilter] = useState<'ALL' | 'POKEMON' | 'YUGIOH'>('ALL')

  const library = useLibrary({ type: 'CARD' })
  const items: LibraryItem[] = library.data ?? []

  const pokemonCount = useMemo(() => items.filter((c) => c.externalId.startsWith('ptcg:')).length, [items])
  const yugiohCount = useMemo(() => items.filter((c) => c.externalId.startsWith('ygo:')).length, [items])

  const toggleSection = (key: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })

  const visible = useMemo(() => {
    let list = items
    if (cardFilter === 'POKEMON') list = list.filter((c) => c.externalId.startsWith('ptcg:'))
    else if (cardFilter === 'YUGIOH') list = list.filter((c) => c.externalId.startsWith('ygo:'))

    if (!search.trim()) return list
    const q = search.toLowerCase()
    return list.filter(
      (i) =>
        i.title.toLowerCase().includes(q) ||
        i.platform.toLowerCase().includes(q) ||
        i.setCode.toLowerCase().includes(q) ||
        i.artist.toLowerCase().includes(q),
    )
  }, [items, search, cardFilter])

  const selected = items.find((i) => i.id === selectedId) ?? null

  return (
    <section>
      <h1>Trading Cards</h1>
      <p className="muted">View and manage your tracked trading card collection.</p>

      {/* Search & Filter Bar */}
      {items.length > 0 && (
        <div style={{ display: 'flex', gap: '0.75rem', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', marginBottom: '1.25rem' }}>
          <div className="searchbar" style={{ margin: 0, flex: 1, minWidth: '220px' }}>
            <input
              type="search"
              placeholder="Search cards by name, set, or code…"
              aria-label="Search cards"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>

          <div style={{ display: 'flex', gap: '0.35rem', alignItems: 'center', flexWrap: 'wrap' }}>
            <button
              type="button"
              onClick={() => setCardFilter('ALL')}
              style={{
                padding: '0.35rem 0.75rem',
                borderRadius: '9999px',
                fontSize: '0.825rem',
                fontWeight: 600,
                cursor: 'pointer',
                border: cardFilter === 'ALL' ? '1px solid var(--accent, #6366f1)' : '1px solid var(--border)',
                backgroundColor: cardFilter === 'ALL' ? 'var(--accent, #6366f1)' : 'var(--surface-alt)',
                color: cardFilter === 'ALL' ? '#fff' : 'var(--text)',
                transition: 'all 0.15s ease',
              }}
            >
              All ({items.length})
            </button>
            <button
              type="button"
              onClick={() => setCardFilter('POKEMON')}
              style={{
                padding: '0.35rem 0.75rem',
                borderRadius: '9999px',
                fontSize: '0.825rem',
                fontWeight: 600,
                cursor: 'pointer',
                border: cardFilter === 'POKEMON' ? '1px solid var(--accent, #6366f1)' : '1px solid var(--border)',
                backgroundColor: cardFilter === 'POKEMON' ? 'var(--accent, #6366f1)' : 'var(--surface-alt)',
                color: cardFilter === 'POKEMON' ? '#fff' : 'var(--text)',
                transition: 'all 0.15s ease',
              }}
            >
              ⚡ Pokémon ({pokemonCount})
            </button>
            <button
              type="button"
              onClick={() => setCardFilter('YUGIOH')}
              style={{
                padding: '0.35rem 0.75rem',
                borderRadius: '9999px',
                fontSize: '0.825rem',
                fontWeight: 600,
                cursor: 'pointer',
                border: cardFilter === 'YUGIOH' ? '1px solid var(--accent, #6366f1)' : '1px solid var(--border)',
                backgroundColor: cardFilter === 'YUGIOH' ? 'var(--accent, #6366f1)' : 'var(--surface-alt)',
                color: cardFilter === 'YUGIOH' ? '#fff' : 'var(--text)',
                transition: 'all 0.15s ease',
              }}
            >
              ⚔️ Yu-Gi-Oh! ({yugiohCount})
            </button>
          </div>
        </div>
      )}

      {library.isPending && <p className="muted">Loading your cards…</p>}
      {library.isError && (
        <p role="alert" className="alert">
          {library.error instanceof ApiError
            ? library.error.message
            : 'Could not load your card collection. Try refreshing.'}
        </p>
      )}

      {library.data !== undefined && items.length === 0 && (
        <p className="empty">
          No cards yet. Photograph one from the Scan page to start your card collection.
        </p>
      )}

      {items.length > 0 && visible.length === 0 && (
        <p className="empty">No cards match your search.</p>
      )}

      {visible.length > 0 &&
        groupByGameAndSet(visible).map(({ game, sets }) => (
          <section key={game} aria-label={game} style={{ marginBottom: '1.5rem' }}>
            <h2>{game}</h2>
            {sets.map(({ set, cards }) => {
              const sectionKey = `${game} · ${set}`
              const open = !collapsed.has(sectionKey)
              const setValue = cards.reduce((sum, c) => sum + (c.price || 0), 0)
              return (
                <section key={set} className="library-section">
                  <button
                    type="button"
                    className="library-section-title"
                    aria-expanded={open}
                    onClick={() => toggleSection(sectionKey)}
                  >
                    <span className="show-caret" aria-hidden="true">
                      {open ? '▾' : '▸'}
                    </span>
                    {set}{' '}
                    <span className="badge">
                      {cards.length} · {formatUsd(setValue)}
                    </span>
                  </button>
                  {open && (
                    <ul className="cover-grid">
                      {cards.map((item) => (
                        <li key={item.id}>
                          <button
                            type="button"
                            className="cover-tile"
                            aria-label={`Open ${item.title}`}
                            onClick={() => setSelectedId(item.id)}
                          >
                            <Poster posterPath={item.artworkPath} title={item.title} width={140} height={195} />
                            <span className="cover-title">{item.title}</span>
                            {item.setCode !== '' && <span className="meta">{item.setCode}</span>}
                            {item.price > 0 && <span className="meta">{formatUsd(item.price)}</span>}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </section>
              )
            })}
          </section>
        ))}

      {selected !== null && (
        <LibraryDetail item={selected} onClose={() => setSelectedId(null)} />
      )}
    </section>
  )
}
