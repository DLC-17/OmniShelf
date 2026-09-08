import { ApiError } from '../api/client'
import { useUpcoming } from '../hooks/useUpcoming'
import Poster from '../components/tv/Poster'

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

export default function Upcoming() {
  const upcoming = useUpcoming()

  if (upcoming.isPending) {
    return <p className="muted" style={{ padding: '2rem' }}>Loading upcoming releases…</p>
  }

  if (upcoming.isError) {
    return (
      <p role="alert" className="alert" style={{ margin: '2rem' }}>
        {upcoming.error instanceof ApiError
          ? upcoming.error.message
          : 'Could not load upcoming releases. Try refreshing.'}
      </p>
    )
  }

  const { tv, movies, games, books } = upcoming.data

  const renderSection = (title: string, items: typeof tv, emptyMsg: string) => (
    <div style={{ flex: '1 1 300px', minWidth: 300 }}>
      <h2>{title}</h2>
      {items.length === 0 ? (
        <p className="empty">{emptyMsg}</p>
      ) : (
        <ul className="list">
          {items.map((item, i) => {
            const rel = relativeDays(item.date)
            return (
              <li key={`${item.title}-${item.date}-${i}`} className="card">
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
          })}
        </ul>
      )}
    </div>
  )

  return (
    <main style={{ padding: '2rem' }}>
      <h1>Upcoming Releases</h1>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: '2rem' }}>
        {renderSection('TV Shows', tv, 'No upcoming episodes.')}
        {renderSection('Movies', movies, 'No upcoming movies.')}
        {renderSection('Games', games, 'No upcoming games.')}
        {renderSection('Books', books, 'No upcoming books.')}
      </div>
    </main>
  )
}
