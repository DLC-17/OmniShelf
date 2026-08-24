import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getTimeSpent, getHeatmap, getBadges } from '../api/stats'
import type { TimeSpentStats, HeatmapEntry, Badge } from '../api/stats'
import { useLibrary } from '../hooks/useLibrary'
import type { LibraryItem } from '../api/library'
import { formatUsd } from '../lib/currency'

function formatMinutes(mins: number): string {
  const h = Math.floor(mins / 60)
  const m = mins % 60
  if (h === 0) return `${m}m`
  return m > 0 ? `${h}h ${m}m` : `${h}h`
}

interface MonthDay {
  date: string
  count: number
  dayNumber: number
}

function getHeatmapColor(count: number): { bg: string; border: string } {
  if (count <= 0) {
    return { bg: 'var(--surface-alt)', border: 'var(--border)' }
  }
  if (count === 1) {
    return {
      bg: 'color-mix(in srgb, #E2B48C 35%, var(--surface-alt))',
      border: 'color-mix(in srgb, #E2B48C 50%, var(--border))',
    }
  }
  if (count === 2) {
    return {
      bg: 'color-mix(in srgb, #E2B48C 70%, var(--surface-alt))',
      border: '#E2B48C',
    }
  }
  if (count === 3) {
    return {
      bg: '#E2B48C',
      border: '#D49D70',
    }
  }
  return {
    bg: '#C89466',
    border: '#B58052',
  }
}

function formatDateFriendly(dateStr: string): string {
  const [y, m, d] = dateStr.split('-').map(Number)
  if (!y || !m || !d) return dateStr
  const dateObj = new Date(y, m - 1, d)
  return dateObj.toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

export function Stats() {
  const [hoveredDay, setHoveredDay] = useState<MonthDay | null>(null)

  const {
    data: timeSpent,
    isLoading: isLoadingTime,
    error: timeError,
  } = useQuery<TimeSpentStats, Error>({
    queryKey: ['timeSpent'],
    queryFn: getTimeSpent,
  })

  const {
    data: heatmap,
    isLoading: isLoadingHeatmap,
    error: heatmapError,
  } = useQuery<HeatmapEntry[], Error>({
    queryKey: ['heatmap'],
    queryFn: getHeatmap,
  })

  const {
    data: badges,
    isLoading: isLoadingBadges,
    error: badgesError,
  } = useQuery<Badge[], Error>({
    queryKey: ['badges'],
    queryFn: getBadges,
  })

  // Trading Cards Collection Data
  const [cardFilter, setCardFilter] = useState<'ALL' | 'POKEMON' | 'YUGIOH'>('ALL')
  const cardsLibrary = useLibrary({ type: 'CARD' })
  const cards: LibraryItem[] = cardsLibrary.data ?? []

  const pokemonStats = useMemo(() => {
    const list = cards.filter((c) => c.externalId.startsWith('ptcg:'))
    return {
      count: list.length,
      value: list.reduce((acc, c) => acc + (c.price || 0), 0),
    }
  }, [cards])

  const yugiohStats = useMemo(() => {
    const list = cards.filter((c) => c.externalId.startsWith('ygo:'))
    return {
      count: list.length,
      value: list.reduce((acc, c) => acc + (c.price || 0), 0),
    }
  }, [cards])

  const filteredCards = useMemo(() => {
    if (cardFilter === 'POKEMON') return cards.filter((c) => c.externalId.startsWith('ptcg:'))
    if (cardFilter === 'YUGIOH') return cards.filter((c) => c.externalId.startsWith('ygo:'))
    return cards
  }, [cards, cardFilter])

  const filteredValuation = useMemo(() => {
    return filteredCards.reduce((acc, item) => acc + (item.price || 0), 0)
  }, [filteredCards])

  const filteredTopCard = useMemo(() => {
    if (filteredCards.length === 0) return null
    return [...filteredCards].sort((a, b) => (b.price || 0) - (a.price || 0))[0]
  }, [filteredCards])

  // Build horizontal month days array (Day 1 through end of current month)
  const calendarData = useMemo(() => {
    const countMap = new Map<string, number>()
    if (heatmap) {
      for (const entry of heatmap) {
        countMap.set(entry.date, entry.count)
      }
    }

    const today = new Date()
    const year = today.getFullYear()
    const month = today.getMonth() // 0-indexed
    const monthName = today.toLocaleString('default', { month: 'long', year: 'numeric' })

    const lastDay = new Date(year, month + 1, 0).getDate()
    const days: MonthDay[] = []

    for (let dayNum = 1; dayNum <= lastDay; dayNum++) {
      const dateStr = `${year}-${String(month + 1).padStart(2, '0')}-${String(dayNum).padStart(2, '0')}`
      const count = countMap.get(dateStr) ?? 0

      days.push({
        date: dateStr,
        count,
        dayNumber: dayNum,
      })
    }

    return { monthName, days }
  }, [heatmap])

  if (isLoadingTime || isLoadingHeatmap || isLoadingBadges) {
    return <p className="muted" style={{ padding: '1rem 0' }}>Loading activity stats…</p>
  }

  if (timeError || heatmapError || badgesError) {
    return <p role="alert" className="alert" style={{ margin: '1rem 0' }}>Could not load activity stats.</p>
  }

  const totalMinutes =
    (timeSpent?.minutesTv ?? 0) +
    (timeSpent?.minutesMovie ?? 0) +
    (timeSpent?.minutesBook ?? 0) +
    (timeSpent?.minutesGame ?? 0)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem', marginTop: '1.5rem' }}>
      {/* Top Stats Grid: Time Spent (Left) & Collection Value (Right), Stacking Vertically on Mobile */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))',
          gap: '1.5rem',
          alignItems: 'stretch',
        }}
      >
        {/* Time Spent Section */}
        <div className="card" style={{ margin: 0, display: 'flex', flexDirection: 'column' }}>
          <h2>Time Spent</h2>
          {timeSpent ? (
            <>
              <p style={{ fontSize: '1.2rem', fontWeight: 'bold', marginBottom: '1rem' }}>
                Total: {formatMinutes(totalMinutes)}
              </p>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: '0.75rem', marginTop: 'auto' }}>
                <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                  <span style={{ fontSize: '0.85rem' }} className="muted">📺 TV Shows</span>
                  <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>{formatMinutes(timeSpent.minutesTv)}</p>
                </div>
                <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                  <span style={{ fontSize: '0.85rem' }} className="muted">🎬 Movies</span>
                  <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>{formatMinutes(timeSpent.minutesMovie)}</p>
                </div>
                <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                  <span style={{ fontSize: '0.85rem' }} className="muted">📚 Books</span>
                  <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>{formatMinutes(timeSpent.minutesBook)}</p>
                </div>
                <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                  <span style={{ fontSize: '0.85rem' }} className="muted">🎮 Games</span>
                  <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>{formatMinutes(timeSpent.minutesGame)}</p>
                </div>
              </div>
            </>
          ) : (
            <p className="muted">No time spent data available.</p>
          )}
        </div>

        {/* Collection Value Section with Interactive TCG Filter Chips */}
        <div className="card" style={{ margin: 0, display: 'flex', flexDirection: 'column' }}>
          <h2>Collection Value</h2>
          {cards.length > 0 ? (
            <>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.5rem', marginBottom: '0.75rem' }}>
                <p style={{ fontSize: '1.2rem', fontWeight: 'bold', margin: 0 }}>
                  Total: {formatUsd(filteredValuation)}
                </p>
                {/* TCG Selection Chips */}
                <div style={{ display: 'flex', gap: '0.35rem', alignItems: 'center', flexWrap: 'wrap' }}>
                  <button
                    type="button"
                    onClick={() => setCardFilter('ALL')}
                    style={{
                      padding: '0.2rem 0.6rem',
                      borderRadius: '9999px',
                      fontSize: '0.75rem',
                      fontWeight: 600,
                      cursor: 'pointer',
                      border: cardFilter === 'ALL' ? '1px solid var(--accent, #6366f1)' : '1px solid var(--border)',
                      backgroundColor: cardFilter === 'ALL' ? 'var(--accent, #6366f1)' : 'var(--surface-alt)',
                      color: cardFilter === 'ALL' ? '#fff' : 'var(--text)',
                      transition: 'all 0.15s ease',
                    }}
                  >
                    All ({cards.length})
                  </button>
                  <button
                    type="button"
                    onClick={() => setCardFilter('POKEMON')}
                    style={{
                      padding: '0.2rem 0.6rem',
                      borderRadius: '9999px',
                      fontSize: '0.75rem',
                      fontWeight: 600,
                      cursor: 'pointer',
                      border: cardFilter === 'POKEMON' ? '1px solid var(--accent, #6366f1)' : '1px solid var(--border)',
                      backgroundColor: cardFilter === 'POKEMON' ? 'var(--accent, #6366f1)' : 'var(--surface-alt)',
                      color: cardFilter === 'POKEMON' ? '#fff' : 'var(--text)',
                      transition: 'all 0.15s ease',
                    }}
                  >
                    ⚡ Pokémon ({pokemonStats.count})
                  </button>
                  <button
                    type="button"
                    onClick={() => setCardFilter('YUGIOH')}
                    style={{
                      padding: '0.2rem 0.6rem',
                      borderRadius: '9999px',
                      fontSize: '0.75rem',
                      fontWeight: 600,
                      cursor: 'pointer',
                      border: cardFilter === 'YUGIOH' ? '1px solid var(--accent, #6366f1)' : '1px solid var(--border)',
                      backgroundColor: cardFilter === 'YUGIOH' ? 'var(--accent, #6366f1)' : 'var(--surface-alt)',
                      color: cardFilter === 'YUGIOH' ? '#fff' : 'var(--text)',
                      transition: 'all 0.15s ease',
                    }}
                  >
                    ⚔️ Yu-Gi-Oh! ({yugiohStats.count})
                  </button>
                </div>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: '0.75rem', marginTop: 'auto' }}>
                {cardFilter === 'ALL' ? (
                  <>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">⚡ Pokémon</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {formatUsd(pokemonStats.value)} <span className="muted" style={{ fontSize: '0.8rem', fontWeight: 'normal' }}>({pokemonStats.count})</span>
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">⚔️ Yu-Gi-Oh!</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {formatUsd(yugiohStats.value)} <span className="muted" style={{ fontSize: '0.8rem', fontWeight: 'normal' }}>({yugiohStats.count})</span>
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">💎 Top Card</span>
                      <p
                        style={{ margin: '0.25rem 0 0', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        title={filteredTopCard ? `${filteredTopCard.title} (${formatUsd(filteredTopCard.price)})` : 'None'}
                      >
                        {filteredTopCard ? formatUsd(filteredTopCard.price) : '$0.00'}
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">📦 Total Tracked</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {cards.length} card{cards.length === 1 ? '' : 's'}
                      </p>
                    </div>
                  </>
                ) : cardFilter === 'POKEMON' ? (
                  <>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">⚡ Total Value</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {formatUsd(pokemonStats.value)}
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">🃏 Tracked Cards</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {pokemonStats.count} card{pokemonStats.count === 1 ? '' : 's'}
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">💎 Top Pokémon</span>
                      <p
                        style={{ margin: '0.25rem 0 0', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        title={filteredTopCard ? `${filteredTopCard.title} (${formatUsd(filteredTopCard.price)})` : 'None'}
                      >
                        {filteredTopCard ? formatUsd(filteredTopCard.price) : '$0.00'}
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">📊 Avg Card Value</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {pokemonStats.count > 0 ? formatUsd(pokemonStats.value / pokemonStats.count) : '$0.00'}
                      </p>
                    </div>
                  </>
                ) : (
                  <>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">⚔️ Total Value</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {formatUsd(yugiohStats.value)}
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">🃏 Tracked Cards</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {yugiohStats.count} card{yugiohStats.count === 1 ? '' : 's'}
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">💎 Top Yu-Gi-Oh!</span>
                      <p
                        style={{ margin: '0.25rem 0 0', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        title={filteredTopCard ? `${filteredTopCard.title} (${formatUsd(filteredTopCard.price)})` : 'None'}
                      >
                        {filteredTopCard ? formatUsd(filteredTopCard.price) : '$0.00'}
                      </p>
                    </div>
                    <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                      <span style={{ fontSize: '0.85rem' }} className="muted">📊 Avg Card Value</span>
                      <p style={{ margin: '0.25rem 0 0', fontWeight: 600 }}>
                        {yugiohStats.count > 0 ? formatUsd(yugiohStats.value / yugiohStats.count) : '$0.00'}
                      </p>
                    </div>
                  </>
                )}
              </div>
            </>
          ) : (
            <p className="muted">No trading cards tracked yet.</p>
          )}
        </div>
      </div>

      {/* Activity Heatmap Section - 10 Days per Row on Large Screens, Horizontally Aligned & Wrapping */}
      <div className="card">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem', flexWrap: 'wrap', gap: '0.5rem' }}>
          <h2 style={{ margin: 0 }}>Activity Heatmap</h2>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            {hoveredDay && (
              <span className="badge" style={{ backgroundColor: 'var(--surface-alt)', borderColor: '#E2B48C', color: 'var(--text)' }}>
                {formatDateFriendly(hoveredDay.date)}: <strong>{hoveredDay.count}</strong> item{hoveredDay.count === 1 ? '' : 's'}
              </span>
            )}
            <span className="muted" style={{ fontSize: '0.85rem', fontWeight: 600 }}>
              {calendarData.monthName}
            </span>
          </div>
        </div>

        <div style={{ width: '100%' }}>
          {/* Horizontally aligned grid: 10 columns per row on desktop, wrapping on smaller screens */}
          <div className="heatmap-month-grid">
            {calendarData.days.map((day) => {
              const { bg, border } = getHeatmapColor(day.count)
              const isHovered = hoveredDay?.date === day.date
              return (
                <div
                  key={day.date}
                  className="heatmap-cell"
                  title={`${formatDateFriendly(day.date)}: ${day.count} logged item${day.count === 1 ? '' : 's'}`}
                  onMouseEnter={() => setHoveredDay(day)}
                  onMouseLeave={() => setHoveredDay(null)}
                  style={{
                    width: '100%',
                    aspectRatio: '1',
                    backgroundColor: bg,
                    border: `1px solid ${isHovered ? 'var(--text)' : border}`,
                    borderRadius: '4px',
                    boxSizing: 'border-box',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: '0.75rem',
                    color: day.count > 0 ? '#333' : 'var(--muted)',
                    fontWeight: 500,
                  }}
                >
                  {day.dayNumber}
                </div>
              )
            })}
          </div>
        </div>
      </div>

      {/* Badges Section */}
      <div className="card">
        <h2>Your Badges</h2>
        {badges && badges.length > 0 ? (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: '1rem', marginTop: '1rem' }}>
            {badges.map((badge) => (
              <div
                key={badge.id}
                style={{
                  padding: '1rem',
                  border: '1px solid var(--border)',
                  borderRadius: 'var(--radius-sm)',
                  textAlign: 'center',
                  backgroundColor: 'var(--surface-alt)',
                }}
              >
                <div style={{ fontSize: '40px', marginBottom: '0.5rem' }}>{badge.icon}</div>
                <h3 style={{ margin: '0 0 0.25rem', fontSize: '1.05rem' }}>{badge.name}</h3>
                <p className="muted" style={{ fontSize: '0.85rem', margin: '0 0 0.5rem' }}>{badge.description}</p>
                <small className="meta">Earned: {new Date(badge.earnedAt).toLocaleDateString()}</small>
              </div>
            ))}
          </div>
        ) : (
          <p className="muted">No badges earned yet. Keep reading and watching!</p>
        )}
      </div>
    </div>
  )
}

export default Stats
