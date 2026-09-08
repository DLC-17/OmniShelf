import { useState } from 'react'
import { ApiError } from '../../api/client'
import type { UpcomingItem, UpcomingMediaType } from '../../api/upcoming'
import { useUpcoming } from '../../hooks/useUpcoming'
import Poster from './Poster'

const TABS: { value: UpcomingMediaType; label: string }[] = [
  { value: 'tv', label: 'TV' },
  { value: 'movies', label: 'Movies' },
  { value: 'games', label: 'Games' },
  { value: 'books', label: 'Books' },
]

/**
 * Per-tab empty copy. TV and Movies genuinely have no future items; Games and
 * Books have no release-date data at all (they are scan-based, already-released
 * media), so their message explains that rather than implying "check back".
 */
const EMPTY_MESSAGE: Record<UpcomingMediaType, string> = {
  tv: 'No upcoming episodes for the shows you’re watching or have completed. New seasons will appear here once they’re announced.',
  movies: 'No upcoming releases among the movies you track. Add an unreleased movie and it’ll show up here.',
  games: 'No upcoming releases among the games you track. Scan an unreleased game and it’ll show up here.',
  books: 'Release dates aren’t tracked for books yet — they’re added by scanning books you already own.',
}

/** "Wed, Jun 25, 2026" — the release/air date, parsed as a local calendar day. */
function formatDate(iso: string): string {
  const [y, m, d] = iso.split('-').map(Number)
  if (!y || !m || !d) return iso
  return new Date(y, m - 1, d).toLocaleDateString(undefined, {
    weekday: 'short',
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

/** "in 12 days" / "tomorrow" / "today" relative to now, or "" when in the past. */
function relativeDays(iso: string): string {
  const [y, m, d] = iso.split('-').map(Number)
  if (!y || !m || !d) return ''
  const target = new Date(y, m - 1, d)
  const now = new Date()
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const days = Math.round((target.getTime() - startOfToday.getTime()) / 86_400_000)
  if (days < 0) return ''
  if (days === 0) return 'today'
  if (days === 1) return 'tomorrow'
  if (days < 30) return `in ${days} days`
  const weeks = Math.round(days / 7)
  if (days < 90) return `in ${weeks} weeks`
  const months = Math.round(days / 30)
  return `in ${months} months`
}

function UpcomingRow({ item }: { item: UpcomingItem }) {
  const rel = relativeDays(item.date)
  return (
    <li className="card">
      <div className="show-card-row">
        <div className="show-card-poster">
          <Poster
            posterPath={item.posterPath}
            title={item.title}
            style={{ width: '100%', height: '100%', objectFit: 'cover' }}
          />
        </div>
        <div className="show-card-content">
          <h3 style={{ margin: '0 0 0.25rem', fontSize: '1.05rem', lineHeight: 1.25 }}>{item.title}</h3>
          {item.detail !== '' && (
            <p style={{ margin: '0 0 0.25rem', fontWeight: 550, color: 'var(--text)' }}>
              {item.detail}
            </p>
          )}
          <p className="meta" style={{ fontSize: '0.82rem', margin: 0 }}>
            {formatDate(item.date)}
            {rel !== '' && <span className="tag" style={{ marginLeft: '0.4rem' }}>{rel}</span>}
          </p>
        </div>
      </div>
    </li>
  )
}

/**
 * Upcoming releases board: what the media you follow will release next, with a
 * tab per media type. TV shows future episodes; Movies show future release
 * dates; Games and Books render explanatory empty states (no dates cached).
 */
export default function UpcomingReleases() {
  const [tab, setTab] = useState<UpcomingMediaType>('tv')
  const upcoming = useUpcoming()
  const items = upcoming.data?.[tab] ?? []

  return (
    <section aria-label="Upcoming releases">
      <h2>Upcoming</h2>
      <div className="tabs" role="tablist" aria-label="Upcoming media type">
        {TABS.map((t) => (
          <button
            key={t.value}
            type="button"
            role="tab"
            aria-selected={tab === t.value}
            className={tab === t.value ? 'tab active' : 'tab'}
            onClick={() => setTab(t.value)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {upcoming.isPending && <p className="muted">Loading upcoming releases…</p>}
      {upcoming.isError && (
        <p role="alert" className="alert">
          {upcoming.error instanceof ApiError
            ? upcoming.error.message
            : 'Could not load upcoming releases. Try refreshing.'}
        </p>
      )}

      {upcoming.data !== undefined && items.length === 0 && (
        <p className="empty">{EMPTY_MESSAGE[tab]}</p>
      )}

      {items.length > 0 && (
        <ul className="list">
          {items.map((item, i) => (
            <UpcomingRow key={`${item.title}-${item.date}-${i}`} item={item} />
          ))}
        </ul>
      )}
    </section>
  )
}
