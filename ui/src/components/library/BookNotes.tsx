import { useState } from 'react'
import { ApiError } from '../../api/client'
import { useAddNote, useDeleteNote, useNotes } from '../../hooks/useBookNotes'

export const SENTIMENT_CHIPS = [
  '🤯 Mind-blowing',
  '☕ Cozy',
  '😭 Tear-jerker',
  '⭐ Instant Classic',
  '🧠 Thought-provoking',
]

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
 * Cross-media journal & sentiment reaction: a list of the user's timestamped
 * note entries with optional sentiment reaction pills, an add-note box with
 * clickable sentiment chips, and per-entry delete.
 */
export default function BookNotes({ itemId }: BookNotesProps) {
  const notes = useNotes(itemId)
  const add = useAddNote(itemId)
  const remove = useDeleteNote(itemId)
  const [draft, setDraft] = useState('')
  const [selectedSentiment, setSelectedSentiment] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const toggleSentiment = (sentiment: string) => {
    setSelectedSentiment((prev) => (prev === sentiment ? null : sentiment))
  }

  const submit = () => {
    const body = draft.trim()
    if (body === '' && !selectedSentiment) return
    setError(null)
    add.mutate(
      {
        body: body !== '' ? body : selectedSentiment ?? '',
        sentiment: selectedSentiment ?? undefined,
      },
      {
        onSuccess: () => {
          setDraft('')
          setSelectedSentiment(null)
        },
        onError: (err) =>
          setError(err instanceof ApiError ? err.message : 'Could not add the journal entry. Try again.'),
      },
    )
  }

  const handleDelete = (noteId: number) => {
    setError(null)
    remove.mutate(noteId, {
      onError: (err) =>
        setError(err instanceof ApiError ? err.message : 'Could not delete the journal entry. Try again.'),
    })
  }

  return (
    <div className="detail-summary">
      <h3>Journal & Reactions</h3>

      {/* Sentiment reaction chips */}
      <div style={{ marginBottom: '0.6rem' }}>
        <span className="muted" style={{ fontSize: '0.85rem', display: 'block', marginBottom: '0.35rem' }}>
          Emotional Reaction
        </span>
        <div className="tag-pill-rail" style={{ marginTop: 0 }}>
          {SENTIMENT_CHIPS.map((chip) => {
            const active = selectedSentiment === chip
            return (
              <button
                key={chip}
                type="button"
                className={`tag-pill ${active ? 'active' : ''}`}
                onClick={() => toggleSentiment(chip)}
                disabled={add.isPending}
                aria-pressed={active}
              >
                {chip}
              </button>
            )
          })}
        </div>
      </div>

      <div className="field" style={{ width: '100%' }}>
        <textarea
          aria-label="Add a journal note"
          placeholder="Add thoughts, reflections, or notes…"
          rows={3}
          value={draft}
          disabled={add.isPending}
          onChange={(e) => setDraft(e.target.value)}
          style={{ width: '100%' }}
        />
      </div>
      <div className="cluster" style={{ marginTop: '0.4rem' }}>
        <button
          type="button"
          className="btn-ghost"
          onClick={submit}
          disabled={add.isPending || (draft.trim() === '' && !selectedSentiment)}
        >
          {add.isPending ? 'Adding…' : 'Add journal entry'}
        </button>
      </div>

      {error !== null && (
        <p role="alert" className="alert" style={{ marginTop: '0.5rem' }}>
          {error}
        </p>
      )}

      {notes.isLoading && <p className="muted" style={{ marginTop: '0.5rem' }}>Loading journal…</p>}
      {notes.isError && <p className="muted" style={{ marginTop: '0.5rem' }}>Could not load journal entries.</p>}
      {notes.data && notes.data.length === 0 && <p className="muted" style={{ marginTop: '0.5rem' }}>No journal entries yet.</p>}

      {notes.data && notes.data.length > 0 && (
        <ul className="note-list">
          {notes.data.map((note) => (
            <li key={note.id} className="note-entry">
              <div className="note-head">
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', flexWrap: 'wrap' }}>
                  <span className="meta">{formatWhen(note.createdAt)}</span>
                  {note.sentiment && (
                    <span
                      className="badge"
                      style={{
                        backgroundColor: 'var(--surface-alt)',
                        borderColor: 'var(--accent)',
                        fontSize: '0.78rem',
                      }}
                    >
                      {note.sentiment}
                    </span>
                  )}
                </div>
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

