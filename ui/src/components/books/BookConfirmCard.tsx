import { useState } from 'react'
import { ApiError } from '../../api/client'
import { trackBook } from '../../api/books'
import type { Book, BookStatus } from '../../api/books'

interface BookConfirmCardProps {
  book: Book
  /** Reset back to the scanner/manual entry to add another book. */
  onDone: () => void
}

const STATUS_LABELS: Record<BookStatus, string> = {
  READING: 'Reading',
  PLAN_TO: 'Plan to read',
  COMPLETED: 'Completed',
}

/**
 * Confirm card for a scanned book (spec §2.5 step 4): cover, title, author and a
 * status choice, then POST /api/books/track. A 409 already_tracked is reported
 * as an informational message rather than a hard error (E16).
 */
export default function BookConfirmCard({ book, onDone }: BookConfirmCardProps) {
  const [status, setStatus] = useState<BookStatus>('READING')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [tracked, setTracked] = useState(false)

  const handleTrack = async () => {
    setError(null)
    setNotice(null)
    setSubmitting(true)
    try {
      await trackBook(book.id, status)
      setTracked(true)
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        // Already on the user's shelf — not a failure, just inform them (E16).
        setNotice(err.message)
        setTracked(true)
      } else if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Something went wrong. Please try again.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  // CoverPath is relative (e.g. "book/<isbn>.jpg") served under /images (E5:
  // may be empty when OpenLibrary had no cover).
  const coverSrc = book.coverPath !== '' ? `/images/${book.coverPath}` : null

  return (
    <section aria-label="Confirm book" style={{ maxWidth: '28rem', margin: '0 auto' }}>
      <div style={{ display: 'flex', gap: '1rem' }}>
        {coverSrc !== null ? (
          <img src={coverSrc} alt={`Cover of ${book.title}`} width={96} height={144} />
        ) : (
          <div
            aria-hidden="true"
            style={{
              width: 96,
              height: 144,
              background: '#e5e7eb',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: '0.75rem',
              color: '#6b7280',
            }}
          >
            No cover
          </div>
        )}
        <div>
          <h2 style={{ margin: '0 0 0.25rem' }}>{book.title}</h2>
          {book.authors !== '' && <p style={{ margin: 0 }}>{book.authors}</p>}
          <p style={{ margin: '0.25rem 0', color: '#6b7280', fontSize: '0.875rem' }}>
            ISBN {book.isbn13}
          </p>
        </div>
      </div>

      {tracked ? (
        <div>
          <p role="status">{notice ?? `Added to your shelf as “${STATUS_LABELS[status]}”.`}</p>
          <button type="button" onClick={onDone}>
            Scan another
          </button>
        </div>
      ) : (
        <div style={{ marginTop: '1rem' }}>
          <label>
            Status{' '}
            <select
              aria-label="Status"
              value={status}
              onChange={(e) => setStatus(e.target.value as BookStatus)}
            >
              {(Object.keys(STATUS_LABELS) as BookStatus[]).map((value) => (
                <option key={value} value={value}>
                  {STATUS_LABELS[value]}
                </option>
              ))}
            </select>
          </label>
          {error !== null && (
            <p role="alert" style={{ color: 'crimson' }}>
              {error}
            </p>
          )}
          <div style={{ marginTop: '0.75rem', display: 'flex', gap: '0.5rem' }}>
            <button type="button" onClick={handleTrack} disabled={submitting}>
              {submitting ? 'Adding…' : 'Add to shelf'}
            </button>
            <button type="button" onClick={onDone} disabled={submitting}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </section>
  )
}
