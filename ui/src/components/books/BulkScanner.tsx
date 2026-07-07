import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import { scanBook, trackBook } from '../../api/books'
import type { BookStatus } from '../../api/books'
import Poster from '../tv/Poster'

const STATUS_LABELS: Record<BookStatus, string> = {
  READING: 'Reading',
  PLAN_TO: 'Plan to read',
  COMPLETED: 'Completed',
}

type Outcome = 'added' | 'exists' | 'notfound' | 'error'

interface ScanRow {
  key: number
  isbn: string
  outcome: Outcome
  title?: string
  authors?: string
  coverPath?: string
  message?: string
}

/**
 * Bulk add books with a handheld/library barcode scanner. A USB scanner acts
 * as a keyboard — it types the ISBN and presses Enter — so this is just a
 * focused text field. Each scan is looked up and added automatically with the
 * chosen shelf status; the field clears and re-focuses so books can be run
 * through one after another. Duplicates and misses are reported inline without
 * interrupting the flow.
 */
export default function BulkScanner() {
  const [status, setStatus] = useState<BookStatus>('READING')
  const [value, setValue] = useState('')
  const [rows, setRows] = useState<ScanRow[]>([])
  const [looking, setLooking] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const keyRef = useRef(0)

  const focusInput = () => inputRef.current?.focus()

  // Focus the field on mount so a scanner can fire straight into it.
  useEffect(() => {
    focusInput()
  }, [])

  const record = (row: Omit<ScanRow, 'key'>) => {
    keyRef.current += 1
    setRows((prev) => [{ key: keyRef.current, ...row }, ...prev])
  }

  const handleSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const isbn = value.trim()
    setValue('')
    focusInput()
    if (isbn === '') return

    setLooking(true)
    try {
      const book = await scanBook(isbn)
      try {
        await trackBook(book.id, status)
        record({ isbn, outcome: 'added', title: book.title, authors: book.authors, coverPath: book.coverPath })
      } catch (err) {
        if (err instanceof ApiError && err.status === 409) {
          record({ isbn, outcome: 'exists', title: book.title, authors: book.authors, coverPath: book.coverPath })
        } else {
          record({ isbn, outcome: 'error', message: err instanceof ApiError ? err.message : 'Could not add this book.' })
        }
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        record({ isbn, outcome: 'notfound' })
      } else {
        record({ isbn, outcome: 'error', message: err instanceof ApiError ? err.message : 'Lookup failed.' })
      }
    } finally {
      setLooking(false)
      focusInput()
    }
  }

  const addedCount = rows.filter((r) => r.outcome === 'added').length

  return (
    <div className="stack">
      <div className="callout">
        <strong>Handheld scanner mode.</strong>
        <p style={{ margin: '0.4rem 0 0' }}>
          Keep the field below focused and scan books one after another — each is added
          automatically. A USB/library scanner types the ISBN and presses Enter for you.
        </p>
      </div>

      <label className="field">
        <span>Add scanned books as</span>
        <select
          aria-label="Shelf for scanned books"
          value={status}
          onChange={(e) => setStatus(e.target.value as BookStatus)}
        >
          {(Object.keys(STATUS_LABELS) as BookStatus[]).map((s) => (
            <option key={s} value={s}>
              {STATUS_LABELS[s]}
            </option>
          ))}
        </select>
      </label>

      <form className="searchbar" onSubmit={handleSubmit}>
        <input
          ref={inputRef}
          type="text"
          inputMode="numeric"
          aria-label="Scan ISBN"
          placeholder="Scan or type an ISBN, then Enter"
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
        <button type="submit" className="btn-primary">
          Add
        </button>
      </form>

      {looking && (
        <p role="status" className="muted">
          Looking up…
        </p>
      )}

      {rows.length > 0 && (
        <>
          <p className="muted">
            {addedCount} added · {rows.length} scanned this session
          </p>
          <ul className="list">
            {rows.map((r) => (
              <li key={r.key} className="card card-row">
                {r.outcome === 'added' || r.outcome === 'exists' ? (
                  <>
                    <Poster posterPath={r.coverPath ?? ''} title={r.title ?? r.isbn} width={40} height={60} />
                    <div className="grow">
                      <strong>{r.title}</strong>
                      {r.authors !== undefined && r.authors !== '' && <p className="meta">{r.authors}</p>}
                    </div>
                    <span className={r.outcome === 'added' ? 'badge badge-ok' : 'badge'}>
                      {r.outcome === 'added' ? 'Added' : 'Already on shelf'}
                    </span>
                  </>
                ) : (
                  <div className="grow">
                    <strong>{r.isbn}</strong>
                    <p className="alert" style={{ margin: 0 }}>
                      {r.outcome === 'notfound' ? 'No book found for this ISBN.' : r.message}
                    </p>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}
