import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import Poster from '../tv/Poster'

/** A metadata result normalized for the bulk list, whatever the media type. */
export interface ScannedMedia {
  id: number
  title: string
  subtitle: string // authors (books) / platform (games)
  coverPath: string
}

/**
 * A media-specific binding for the bulk scanner: how to look a code up, how to
 * shelve it, and the labels/statuses to show. Books and games each supply one.
 */
export interface ScanTarget<S extends string> {
  noun: string // "book" / "game" — used in copy
  codeNoun: string // "ISBN" / "barcode" — the scanned code's name
  inputLabel: string // aria-label for the field, e.g. "Scan ISBN"
  placeholder: string
  statuses: { value: S; label: string }[]
  defaultStatus: S
  scan: (code: string) => Promise<ScannedMedia>
  track: (id: number, status: S) => Promise<unknown>
}

type Outcome = 'added' | 'exists' | 'notfound' | 'error'

interface ScanRow {
  key: number
  code: string
  outcome: Outcome
  title?: string
  subtitle?: string
  coverPath?: string
  message?: string
}

/**
 * Bulk add media with a handheld/library barcode scanner. A USB scanner acts
 * as a keyboard — it types the code and presses Enter — so this is just a
 * focused text field. Each scan is looked up and added automatically with the
 * chosen shelf status; the field clears and re-focuses so items can be run
 * through one after another. Duplicates and misses are reported inline without
 * interrupting the flow. The media type (book or game) is supplied by `target`.
 */
export default function BulkScanner<S extends string>({ target }: { target: ScanTarget<S> }) {
  const [status, setStatus] = useState<S>(target.defaultStatus)
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
    const code = value.trim()
    setValue('')
    focusInput()
    if (code === '') return

    setLooking(true)
    try {
      const media = await target.scan(code)
      try {
        await target.track(media.id, status)
        record({ code, outcome: 'added', title: media.title, subtitle: media.subtitle, coverPath: media.coverPath })
      } catch (err) {
        if (err instanceof ApiError && err.status === 409) {
          record({ code, outcome: 'exists', title: media.title, subtitle: media.subtitle, coverPath: media.coverPath })
        } else {
          record({ code, outcome: 'error', message: err instanceof ApiError ? err.message : `Could not add this ${target.noun}.` })
        }
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        record({ code, outcome: 'notfound' })
      } else {
        record({ code, outcome: 'error', message: err instanceof ApiError ? err.message : 'Lookup failed.' })
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
          Keep the field below focused and scan {target.noun}s one after another — each is added
          automatically. A USB/library scanner types the code and presses Enter for you.
        </p>
      </div>

      <label className="field">
        <span>Add scanned {target.noun}s as</span>
        <select
          aria-label={`Shelf for scanned ${target.noun}s`}
          value={status}
          onChange={(e) => setStatus(e.target.value as S)}
        >
          {target.statuses.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </select>
      </label>

      <form className="searchbar" onSubmit={handleSubmit}>
        <input
          ref={inputRef}
          type="text"
          inputMode="numeric"
          aria-label={target.inputLabel}
          placeholder={target.placeholder}
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
                    <Poster posterPath={r.coverPath ?? ''} title={r.title ?? r.code} width={40} height={60} />
                    <div className="grow">
                      <strong>{r.title}</strong>
                      {r.subtitle !== undefined && r.subtitle !== '' && <p className="meta">{r.subtitle}</p>}
                    </div>
                    <span className={r.outcome === 'added' ? 'badge badge-ok' : 'badge'}>
                      {r.outcome === 'added' ? 'Added' : 'Already on shelf'}
                    </span>
                  </>
                ) : (
                  <div className="grow">
                    <strong>{r.code}</strong>
                    <p className="alert" style={{ margin: 0 }}>
                      {r.outcome === 'notfound' ? `No ${target.noun} found for this ${target.codeNoun}.` : r.message}
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
