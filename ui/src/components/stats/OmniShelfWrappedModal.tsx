import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getWrapped } from '../../api/stats'
import { formatUsd } from '../../lib/currency'

interface OmniShelfWrappedModalProps {
  onClose: () => void
  initialYear?: number
}

export default function OmniShelfWrappedModal({ onClose, initialYear = new Date().getFullYear() }: OmniShelfWrappedModalProps) {
  const [year, setYear] = useState<number>(initialYear)
  const [slide, setSlide] = useState<number>(0)

  const { data: wrapped, isLoading, isError } = useQuery({
    queryKey: ['stats', 'wrapped', year],
    queryFn: () => getWrapped(year),
  })

  const slides = [
    { title: 'Overview', id: 'overview' },
    { title: 'Time Invested', id: 'time' },
    { title: 'Top Genres & Creators', id: 'tastes' },
    { title: 'Peak Season', id: 'season' },
    { title: 'Physical Vault', id: 'physical' },
  ]

  const nextSlide = () => setSlide((prev) => (prev < slides.length - 1 ? prev + 1 : prev))
  const prevSlide = () => setSlide((prev) => (prev > 0 ? prev - 1 : prev))

  return (
    <div className="modal-backdrop" onClick={onClose} role="dialog" aria-modal="true">
      <div
        className="modal-content wrapped-modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{
          maxWidth: '680px',
          width: '90%',
          background: 'linear-gradient(145deg, var(--surface) 0%, var(--surface-alt) 100%)',
          border: '1px solid var(--accent)',
          borderRadius: 'var(--radius-lg)',
          boxShadow: 'var(--shadow-lg)',
          padding: '1.75rem',
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.25rem' }}>
          <div>
            <span className="badge badge-relation" style={{ fontSize: '0.75rem', textTransform: 'uppercase' }}>
              OmniShelf Retrospective
            </span>
            <h2 style={{ margin: '0.25rem 0 0 0', fontSize: '1.5rem' }}>✨ {year} Wrapped</h2>
          </div>
          <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
            <select
              value={year}
              onChange={(e) => {
                setYear(Number(e.target.value))
                setSlide(0)
              }}
              style={{ padding: '0.25rem 0.5rem', fontSize: '0.85rem' }}
            >
              {[2026, 2025, 2024, 2023].map((y) => (
                <option key={y} value={y}>{y}</option>
              ))}
            </select>
            <button type="button" className="btn-secondary" onClick={onClose} style={{ padding: '0.25rem 0.6rem' }}>
              ✕
            </button>
          </div>
        </div>

        {/* Stepper Progress Bar */}
        <div style={{ display: 'flex', gap: '0.35rem', marginBottom: '1.5rem' }}>
          {slides.map((s, idx) => (
            <div
              key={s.id}
              onClick={() => setSlide(idx)}
              style={{
                flex: 1,
                height: '4px',
                borderRadius: '2px',
                cursor: 'pointer',
                background: idx === slide ? 'var(--accent)' : idx < slide ? 'var(--text-muted)' : 'var(--border)',
                transition: 'background 0.2s ease',
              }}
            />
          ))}
        </div>

        {isLoading && <p className="muted" style={{ textAlign: 'center', padding: '2rem' }}>Compiling your year in media…</p>}
        {isError && <p className="muted" style={{ textAlign: 'center', padding: '2rem' }}>Could not compile retrospective for {year}.</p>}

        {wrapped && !isLoading && (
          <div style={{ minHeight: '300px', display: 'flex', flexDirection: 'column', justifyContent: 'space-between' }}>
            {/* Slide 0: Overview */}
            {slide === 0 && (
              <div style={{ textAlign: 'center', padding: '1rem 0' }}>
                <p className="muted" style={{ textTransform: 'uppercase', letterSpacing: '0.08em', fontSize: '0.82rem' }}>
                  Total Works Completed
                </p>
                <div style={{ fontSize: '3.75rem', fontWeight: 800, color: 'var(--accent)', lineHeight: 1.1 }}>
                  {wrapped.totalCompleted}
                </div>
                <p className="muted" style={{ marginBottom: '1.5rem' }}>stories, games & albums conquered in {year}</p>

                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(100px, 1fr))', gap: '0.75rem' }}>
                  <div className="card" style={{ padding: '0.75rem', textAlign: 'center' }}>
                    <div style={{ fontSize: '1.25rem', fontWeight: 700 }}>{wrapped.breakdown.episodes}</div>
                    <small className="muted">📺 TV Episodes</small>
                  </div>
                  <div className="card" style={{ padding: '0.75rem', textAlign: 'center' }}>
                    <div style={{ fontSize: '1.25rem', fontWeight: 700 }}>{wrapped.breakdown.movies}</div>
                    <small className="muted">🎬 Movies</small>
                  </div>
                  <div className="card" style={{ padding: '0.75rem', textAlign: 'center' }}>
                    <div style={{ fontSize: '1.25rem', fontWeight: 700 }}>{wrapped.breakdown.games}</div>
                    <small className="muted">🎮 Games</small>
                  </div>
                  <div className="card" style={{ padding: '0.75rem', textAlign: 'center' }}>
                    <div style={{ fontSize: '1.25rem', fontWeight: 700 }}>{wrapped.breakdown.books}</div>
                    <small className="muted">📚 Books</small>
                  </div>
                  <div className="card" style={{ padding: '0.75rem', textAlign: 'center' }}>
                    <div style={{ fontSize: '1.25rem', fontWeight: 700 }}>{wrapped.breakdown.albums}</div>
                    <small className="muted">🎵 Albums</small>
                  </div>
                </div>
              </div>
            )}

            {/* Slide 1: Time Invested */}
            {slide === 1 && (
              <div style={{ textAlign: 'center', padding: '1rem 0' }}>
                <p className="muted" style={{ textTransform: 'uppercase', letterSpacing: '0.08em', fontSize: '0.82rem' }}>
                  Time Spent In Media
                </p>
                <div style={{ fontSize: '2.5rem', fontWeight: 800, color: 'var(--accent)' }}>
                  {wrapped.timeSpent.formatted || `${wrapped.timeSpent.totalHours} Hours`}
                </div>
                <p className="muted" style={{ marginBottom: '1.5rem' }}>
                  That is approximately <strong>{wrapped.timeSpent.totalDays} full continuous days</strong> of media immersion.
                </p>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem', maxWidth: '420px', margin: '0 auto', textAlign: 'left' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.9rem' }}>
                    <span>📺 TV Watchtime:</span>
                    <strong>{Math.round(wrapped.timeSpent.minutesTv / 60)} hrs</strong>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.9rem' }}>
                    <span>🎬 Movie Screen Time:</span>
                    <strong>{Math.round(wrapped.timeSpent.minutesMovie / 60)} hrs</strong>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.9rem' }}>
                    <span>🎮 Gaming Hours:</span>
                    <strong>{Math.round(wrapped.timeSpent.minutesGame / 60)} hrs</strong>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.9rem' }}>
                    <span>📚 Reading Minutes:</span>
                    <strong>{Math.round(wrapped.timeSpent.minutesBook)} min</strong>
                  </div>
                </div>
              </div>
            )}

            {/* Slide 2: Top Tastes */}
            {slide === 2 && (
              <div style={{ padding: '0.5rem 0' }}>
                <h3>🏆 Signature Creators & Authors</h3>
                <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', marginBottom: '1.25rem' }}>
                  {wrapped.topCreators && wrapped.topCreators.length > 0 ? (
                    wrapped.topCreators.slice(0, 5).map((c, i) => (
                      <span key={c.name} className="badge" style={{ padding: '0.4rem 0.75rem', fontSize: '0.85rem' }}>
                        #{i + 1} {c.name} ({c.count} items)
                      </span>
                    ))
                  ) : (
                    <span className="muted">No creator data logged for {year}.</span>
                  )}
                </div>

                <h3>✨ Favorite Genres</h3>
                <div style={{ display: 'flex', gap: '0.4rem', flexWrap: 'wrap' }}>
                  {wrapped.topGenres && wrapped.topGenres.length > 0 ? (
                    wrapped.topGenres.slice(0, 6).map((g) => (
                      <span key={g.name} className="badge badge-relation" style={{ padding: '0.35rem 0.7rem' }}>
                        {g.name} <strong style={{ marginLeft: '0.35rem' }}>{g.count}</strong>
                      </span>
                    ))
                  ) : (
                    <span className="muted">No genres tagged.</span>
                  )}
                </div>
              </div>
            )}

            {/* Slide 3: Peak Season */}
            {slide === 3 && (
              <div style={{ textAlign: 'center', padding: '1rem 0' }}>
                <p className="muted" style={{ textTransform: 'uppercase', letterSpacing: '0.08em', fontSize: '0.82rem' }}>
                  Your Peak Season
                </p>
                <div style={{ fontSize: '2.5rem', fontWeight: 800, color: 'var(--accent)' }}>
                  🗓️ {wrapped.busiestMonth?.monthName || 'All Year'}
                </div>
                <p className="muted">
                  Your most productive month with <strong>{wrapped.busiestMonth?.count || 0} completed items</strong>.
                </p>

                {/* Monthly micro bar-chart */}
                <div style={{ display: 'flex', alignItems: 'flex-end', height: '100px', gap: '0.4rem', marginTop: '1.5rem', justifyContent: 'center' }}>
                  {wrapped.monthlyHeatmap.map((m) => {
                    const max = Math.max(...wrapped.monthlyHeatmap.map((x) => x.count), 1)
                    const heightPercent = Math.max((m.count / max) * 100, 8)
                    const isBusiest = m.month === wrapped.busiestMonth?.month
                    return (
                      <div key={m.month} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', flex: 1, maxWidth: '32px' }}>
                        <div
                          style={{
                            width: '100%',
                            height: `${heightPercent}px`,
                            background: isBusiest ? 'var(--accent)' : 'var(--surface-alt)',
                            borderRadius: '3px 3px 0 0',
                            border: '1px solid var(--border)',
                          }}
                          title={`${m.monthName}: ${m.count} completions`}
                        />
                        <span style={{ fontSize: '0.65rem', marginTop: '0.25rem' }} className="muted">
                          {m.monthName.slice(0, 3)}
                        </span>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Slide 4: Physical Vault */}
            {slide === 4 && (
              <div style={{ textAlign: 'center', padding: '1rem 0' }}>
                <p className="muted" style={{ textTransform: 'uppercase', letterSpacing: '0.08em', fontSize: '0.82rem' }}>
                  Physical Collection Vault
                </p>
                <div style={{ fontSize: '2.5rem', fontWeight: 800, color: 'var(--accent)' }}>
                  💰 {formatUsd(wrapped.physicalCollection?.totalValue || 0)}
                </div>
                <p className="muted" style={{ marginBottom: '1.25rem' }}>
                  Added to physical catalog across <strong>{wrapped.physicalCollection?.totalItems || 0} items</strong> (Cards, Vinyl, Physical Games).
                </p>

                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: '0.75rem' }}>
                  <div className="card" style={{ padding: '0.75rem', textAlign: 'center' }}>
                    <div style={{ fontWeight: 700 }}>{formatUsd(wrapped.physicalCollection?.cardValue || 0)}</div>
                    <small className="muted">🃏 {wrapped.physicalCollection?.cardCount || 0} Cards</small>
                  </div>
                  <div className="card" style={{ padding: '0.75rem', textAlign: 'center' }}>
                    <div style={{ fontWeight: 700 }}>{formatUsd(wrapped.physicalCollection?.vinylValue || 0)}</div>
                    <small className="muted">🎵 {wrapped.physicalCollection?.vinylCount || 0} Vinyl</small>
                  </div>
                  <div className="card" style={{ padding: '0.75rem', textAlign: 'center' }}>
                    <div style={{ fontWeight: 700 }}>{formatUsd(wrapped.physicalCollection?.gameValue || 0)}</div>
                    <small className="muted">🎮 {wrapped.physicalCollection?.gameCount || 0} Games</small>
                  </div>
                </div>
              </div>
            )}

            {/* Carousel Navigation Buttons */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '1.5rem', paddingTop: '1rem', borderTop: '1px solid var(--border)' }}>
              <button
                type="button"
                className="btn-secondary"
                onClick={prevSlide}
                disabled={slide === 0}
                style={{ visibility: slide === 0 ? 'hidden' : 'visible' }}
              >
                ← Back
              </button>
              <span className="muted" style={{ fontSize: '0.82rem' }}>
                {slide + 1} of {slides.length}
              </span>
              {slide < slides.length - 1 ? (
                <button type="button" className="btn-primary" onClick={nextSlide}>
                  Next →
                </button>
              ) : (
                <button type="button" className="btn-confirm" onClick={onClose}>
                  ✓ Done
                </button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
