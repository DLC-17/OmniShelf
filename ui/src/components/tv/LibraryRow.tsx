import { useState } from 'react'
import { ApiError } from '../../api/client'
import { BOOK_STATUSES, TV_STATUSES } from '../../api/library'
import type { ItemStatus, LibraryItem } from '../../api/library'
import { useDeleteItem, useUpdateItem } from '../../hooks/useLibrary'

interface LibraryRowProps {
  item: LibraryItem
}

/**
 * One library shelf row with inline editing (spec §2.6): a status dropdown for
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
    <li
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '0.75rem',
        flexWrap: 'wrap',
        border: '1px solid #ccc',
        borderRadius: 8,
        padding: '0.5rem 0.75rem',
      }}
    >
      <div style={{ flex: 1, minWidth: '10rem' }}>
        <strong>{item.title}</strong>
        <span style={{ color: '#666', marginLeft: '0.5rem', fontSize: '0.85rem' }}>{item.type}</span>
      </div>

      <label>
        <span style={{ marginRight: '0.25rem' }}>Status</span>
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
        <label>
          <span style={{ marginRight: '0.25rem' }}>Page</span>
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
        <span>
          <span style={{ marginRight: '0.5rem' }}>Remove?</span>
          <button type="button" onClick={handleDelete} disabled={remove.isPending}>
            Confirm
          </button>
          <button type="button" onClick={() => setConfirming(false)} disabled={remove.isPending}>
            Cancel
          </button>
        </span>
      ) : (
        <button
          type="button"
          aria-label={`Delete ${item.title}`}
          onClick={() => setConfirming(true)}
        >
          Delete
        </button>
      )}

      {error !== null && (
        <p role="alert" style={{ color: 'crimson', width: '100%', margin: 0 }}>
          {error}
        </p>
      )}
    </li>
  )
}
