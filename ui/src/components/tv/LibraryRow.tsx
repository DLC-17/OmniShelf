import { useState } from 'react'
import { ApiError } from '../../api/client'
import { BOOK_STATUSES, TV_STATUSES } from '../../api/library'
import type { ItemStatus, LibraryItem } from '../../api/library'
import { useDeleteItem, useUpdateItem } from '../../hooks/useLibrary'

interface LibraryRowProps {
  item: LibraryItem
}

/**
 * One library shelf row with inline editing: a status dropdown for
 * every item, a page-number field for books (TV progress is derived
 * server-side and not editable), and a delete button gated behind a confirm.
 */
export default function LibraryRow({ item }: LibraryRowProps) {
  const update = useUpdateItem()
  const remove = useDeleteItem()
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [progressDraft, setProgressDraft] = useState(String(item.progress))

  const statuses = item.type === 'BOOK' ? BOOK_STATUSES : TV_STATUSES

  const runUpdate = (patch: { status?: ItemStatus; progress?: number }) => {
    setError(null)
    update.mutate(
      { id: item.id, patch },
      {
        onError: (err) => {
          setError(err instanceof ApiError ? err.message : 'Update failed. Try again.')
        },
      },
    )
  }

  const handleStatus = (status: ItemStatus) => {
    if (status !== item.status) runUpdate({ status })
  }

  const commitProgress = () => {
    const parsed = Number.parseInt(progressDraft, 10)
    if (Number.isNaN(parsed) || parsed < 0) {
      setError('Progress must be a non-negative page number.')
      setProgressDraft(String(item.progress))
      return
    }
    if (parsed !== item.progress) runUpdate({ progress: parsed })
  }

  const handleDelete = () => {
    setError(null)
    remove.mutate(item.id, {
      onError: (err) => {
        setError(err instanceof ApiError ? err.message : 'Delete failed. Try again.')
        setConfirming(false)
      },
    })
  }

  return (
    <li className="card card-row wrap">
      <div className="grow" style={{ minWidth: '10rem' }}>
        <strong>{item.title}</strong>
        <span className="tag">{item.type}</span>
      </div>

      <label className="field">
        <span>Status</span>
        <select
          aria-label={`Status for ${item.title}`}
          value={item.status}
          disabled={update.isPending}
          onChange={(e) => handleStatus(e.target.value as ItemStatus)}
        >
          {statuses.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
      </label>

      {item.type === 'BOOK' && (
        <label className="field">
          <span>Page</span>
          <input
            type="number"
            min={0}
            aria-label={`Page for ${item.title}`}
            value={progressDraft}
            disabled={update.isPending}
            onChange={(e) => setProgressDraft(e.target.value)}
            onBlur={commitProgress}
            style={{ width: '5rem' }}
          />
        </label>
      )}

      {confirming ? (
        <span className="cluster">
          <span className="muted">Remove?</span>
          <button type="button" className="btn-danger" onClick={handleDelete} disabled={remove.isPending}>
            Confirm
          </button>
          <button type="button" className="btn-ghost" onClick={() => setConfirming(false)} disabled={remove.isPending}>
            Cancel
          </button>
        </span>
      ) : (
        <button
          type="button"
          className="btn-danger"
          aria-label={`Delete ${item.title}`}
          onClick={() => setConfirming(true)}
        >
          Delete
        </button>
      )}

      {error !== null && (
        <p role="alert" className="alert" style={{ width: '100%', margin: 0 }}>
          {error}
        </p>
      )}
    </li>
  )
}
