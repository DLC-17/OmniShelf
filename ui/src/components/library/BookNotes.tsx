import { useState } from 'react'
import { ApiError } from '../../api/client'
import { useAddNote, useDeleteNote, useNotes } from '../../hooks/useBookNotes'

interface BookNotesProps {
  itemId: number
}

/** Renders a timestamp as a short human-readable local date/time. */
function formatWhen(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

/**
 * Book-only journal: a list of the user's timestamped note entries (newest
 * first), an add-note box, and per-entry delete. Rendered by LibraryDetail only
 * for BOOK items; kept out of the shared layout and other media branches.
 */
export default function BookNotes({ itemId }: BookNotesProps) {
  const notes = useNotes(itemId)
  const add = useAddNote(itemId)
  const remove = useDeleteNote(itemId)
  const [draft, setDraft] = useState('')
  const [error, setError] = useState<string | null>(null)

  const submit = () => {
    const body = draft.trim()
    if (body === '') return
    setError(null)
    add.mutate(body, {
      onSuccess: () => setDraft(''),
      onError: (err) => setError(err instanceof ApiError ? err.message : 'Could not add the note. Try again.'),
    })
  }

  const handleDelete = (noteId: number) => {
    setError(null)
    remove.mutate(noteId, {
      onError: (err) => setError(err instanceof ApiError ? err.message : 'Could not delete the note. Try again.'),
    })
  }

  return (
    <div className="detail-summary">
      <h3>Notes</h3>

      <div className="field">
        <textarea
          aria-label="Add a note"
          placeholder="Add a note…"
          rows={3}
          value={draft}
          disabled={add.isPending}
          onChange={(e) => setDraft(e.target.value)}
        />
      </div>
      <div className="cluster">
        <button
          type="button"
          className="btn-ghost"
          onClick={submit}
          disabled={add.isPending || draft.trim() === ''}
        >
          {add.isPending ? 'Adding…' : 'Add note'}
        </button>
      </div>

      {error !== null && (
        <p role="alert" className="alert">
          {error}
        </p>
      )}

      {notes.isLoading && <p className="muted">Loading notes…</p>}
      {notes.isError && <p className="muted">Could not load notes.</p>}
      {notes.data && notes.data.length === 0 && <p className="muted">No notes yet.</p>}

      {notes.data && notes.data.length > 0 && (
        <ul className="note-list">
          {notes.data.map((note) => (
            <li key={note.id} className="note-entry">
              <div className="note-head">
                <span className="meta">{formatWhen(note.createdAt)}</span>
                <button
                  type="button"
                  className="btn-ghost"
                  aria-label="Delete note"
                  onClick={() => handleDelete(note.id)}
                  disabled={remove.isPending}
                >
                  ✕
                </button>
              </div>
              <p className="note-body">{note.body}</p>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
