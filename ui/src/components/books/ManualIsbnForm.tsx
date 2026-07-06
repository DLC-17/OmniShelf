import { useState } from 'react'
import type { FormEvent } from 'react'

interface ManualIsbnFormProps {
  /** Pre-fills the field — used to seed the scanned ISBN after a 404 (E4). */
  initialIsbn?: string
  /** Called with the trimmed ISBN when the form is submitted. */
  onSubmit: (isbn: string) => void
  /** Disables the field/button while a lookup is in flight. */
  busy?: boolean
}

/**
 * Manual ISBN entry — the always-available fallback for books without a
 * scannable barcode, for insecure contexts (E6) and camera denial (E7), and
 * pre-filled with the scanned ISBN when a scan 404s (E4).
 */
export default function ManualIsbnForm({ initialIsbn = '', onSubmit, busy = false }: ManualIsbnFormProps) {
  const [isbn, setIsbn] = useState(initialIsbn)

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const trimmed = isbn.trim()
    if (trimmed === '') return
    onSubmit(trimmed)
  }

  return (
    <form onSubmit={handleSubmit}>
      <label>
        ISBN{' '}
        <input
          type="text"
          inputMode="numeric"
          aria-label="ISBN"
          placeholder="9780000000000"
          value={isbn}
          onChange={(e) => setIsbn(e.target.value)}
          disabled={busy}
        />
      </label>{' '}
      <button type="submit" disabled={busy || isbn.trim() === ''}>
        {busy ? 'Looking up…' : 'Look up'}
      </button>
    </form>
  )
}
