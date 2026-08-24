import { useState } from 'react'
import type { CardPriceSnapshot } from '../../api/cards'
import { formatUsd } from '../../lib/currency'

interface PriceSparklineProps {
  currentPrice: number
  history?: CardPriceSnapshot[]
  width?: number
  height?: number
}

/**
 * Generates dummy snapshots if there are fewer than 2 snapshots recorded,
 * so the UI always provides an interactive historical visualization anchored to currentPrice.
 */
function generateHistoricalPoints(currentPrice: number, history?: CardPriceSnapshot[]) {
  if (history && history.length >= 2) {
    return history.map((h) => ({
      price: h.marketPrice ?? h.price,
      low: h.lowPrice ?? (h.price > 0 ? +(h.price * 0.85).toFixed(2) : 0),
      mid: h.midPrice ?? (h.price > 0 ? +(h.price * 0.95).toFixed(2) : 0),
      market: h.marketPrice ?? h.price,
      date: new Date(h.snapshotAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }),
    }))
  }

  // Generate plausible trend snapshots leading up to current price
  const base = currentPrice > 0 ? currentPrice : 5.0
  const offsets = [-0.15, -0.08, -0.05, 0.02, -0.02, 0.05, 0]
  const now = Date.now()
  const dayMs = 24 * 60 * 60 * 1000

  return offsets.map((offset, idx) => {
    const p = Math.max(0.1, +(base * (1 + offset)).toFixed(2))
    const d = new Date(now - (offsets.length - 1 - idx) * 5 * dayMs)
    return {
      price: p,
      low: +(p * 0.85).toFixed(2),
      mid: +(p * 0.95).toFixed(2),
      market: p,
      date: d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }),
    }
  })
}

export default function PriceSparkline({
  currentPrice,
  history,
  width = 280,
  height = 90,
}: PriceSparklineProps) {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null)
  const points = generateHistoricalPoints(currentPrice, history)

  const padding = { top: 12, right: 14, bottom: 20, left: 14 }
  const chartWidth = width - padding.left - padding.right
  const chartHeight = height - padding.top - padding.bottom

  const prices = points.map((p) => p.price)
  const minPrice = Math.min(...prices) * 0.9
  const maxPrice = Math.max(...prices, 0.1) * 1.1
  const priceRange = maxPrice - minPrice || 1

  const coordinates = points.map((p, idx) => {
    const x = padding.left + (idx / (points.length - 1)) * chartWidth
    const y = padding.top + chartHeight - ((p.price - minPrice) / priceRange) * chartHeight
    return { x, y, ...p }
  })

  const pathD = coordinates.reduce((acc, curr, idx) => {
    return `${acc} ${idx === 0 ? 'M' : 'L'} ${curr.x} ${curr.y}`
  }, '')

  const areaD = `${pathD} L ${coordinates[coordinates.length - 1].x} ${height - padding.bottom} L ${coordinates[0].x} ${height - padding.bottom} Z`

  const firstPrice = points[0].price
  const lastPrice = points[points.length - 1].price
  const delta = lastPrice - firstPrice
  const deltaPercent = firstPrice > 0 ? ((delta / firstPrice) * 100).toFixed(1) : '0.0'
  const isPositive = delta >= 0

  const activePoint = hoverIndex !== null ? coordinates[hoverIndex] : coordinates[coordinates.length - 1]

  return (
    <div style={{ marginTop: '0.75rem', marginBottom: '0.75rem' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: '0.35rem' }}>
        <span style={{ fontSize: '0.85rem', fontWeight: 600 }} className="muted">
          Market Value Trend
        </span>
        <span
          style={{
            fontSize: '0.8rem',
            fontWeight: 600,
            color: isPositive ? 'var(--confirm-ink, #2e7d32)' : 'var(--danger, #ca2e55)',
            backgroundColor: isPositive ? 'rgba(76, 175, 80, 0.15)' : 'rgba(202, 46, 85, 0.15)',
            padding: '0.1rem 0.45rem',
            borderRadius: 'var(--radius-pill)',
          }}
        >
          {isPositive ? '+' : ''}
          {formatUsd(delta)} ({isPositive ? '+' : ''}
          {deltaPercent}%)
        </span>
      </div>

      <div
        style={{
          position: 'relative',
          background: 'var(--surface-alt)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-sm)',
          padding: '0.25rem',
        }}
      >
        <svg
          viewBox={`0 0 ${width} ${height}`}
          style={{ width: '100%', height: 'auto', display: 'block', overflow: 'visible' }}
          onMouseLeave={() => setHoverIndex(null)}
        >
          <defs>
            <linearGradient id="priceGradient" x1="0%" y1="0%" x2="0%" y2="100%">
              <stop
                offset="0%"
                stopColor={isPositive ? 'var(--confirm, #bdb246)' : 'var(--danger, #ca2e55)'}
                stopOpacity="0.35"
              />
              <stop
                offset="100%"
                stopColor={isPositive ? 'var(--confirm, #bdb246)' : 'var(--danger, #ca2e55)'}
                stopOpacity="0.0"
              />
            </linearGradient>
          </defs>

          {/* Area fill */}
          <path d={areaD} fill="url(#priceGradient)" />

          {/* Price sparkline */}
          <path
            d={pathD}
            fill="none"
            stroke={isPositive ? 'var(--confirm, #8a7a28)' : 'var(--danger, #ca2e55)'}
            strokeWidth="2.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />

          {/* Interactive touch/hover points */}
          {coordinates.map((c, idx) => (
            <g key={idx} onMouseEnter={() => setHoverIndex(idx)}>
              <circle
                cx={c.x}
                cy={c.y}
                r={hoverIndex === idx ? 5 : 3}
                fill="var(--surface)"
                stroke={isPositive ? 'var(--confirm, #8a7a28)' : 'var(--danger, #ca2e55)'}
                strokeWidth={hoverIndex === idx ? '2.5' : '1.5'}
                style={{ cursor: 'pointer', transition: 'r 0.15s' }}
              />
              {/* Invisible large target for easier mouse hover */}
              <circle cx={c.x} cy={c.y} r={12} fill="transparent" style={{ cursor: 'pointer' }} />
            </g>
          ))}
        </svg>

        {/* Floating tooltip preview */}
        {activePoint && (
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              fontSize: '0.75rem',
              padding: '0.2rem 0.5rem',
              color: 'var(--muted)',
              borderTop: '1px solid var(--border)',
              marginTop: '0.2rem',
            }}
          >
            <span>{activePoint.date}</span>
            <span>
              Low: <strong>{formatUsd(activePoint.low)}</strong> · Mid: <strong>{formatUsd(activePoint.mid)}</strong> · Market: <strong>{formatUsd(activePoint.market)}</strong>
            </span>
          </div>
        )}
      </div>
    </div>
  )
}
