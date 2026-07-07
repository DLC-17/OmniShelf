interface RatingStarsProps {
  value: number
  onRate: (rating: number) => void
  busy?: boolean
}

/**
 * Five-star self-rating. Clicking a star sets that rating; clicking the current
 * rating again clears it (back to 0 = unrated).
 */
export default function RatingStars({ value, onRate, busy = false }: RatingStarsProps) {
  return (
    <div className="rating" role="group" aria-label="Your rating">
      {[1, 2, 3, 4, 5].map((n) => (
        <button
          key={n}
          type="button"
          className={n <= value ? 'star on' : 'star'}
          aria-label={`Rate ${n} star${n > 1 ? 's' : ''}`}
          aria-pressed={n === value}
          disabled={busy}
          onClick={() => onRate(n === value ? 0 : n)}
        >
          ★
        </button>
      ))}
    </div>
  )
}
