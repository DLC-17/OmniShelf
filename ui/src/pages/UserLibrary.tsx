import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ApiError } from '../api/client'
import {
  fetchMemberLibrary,
  fetchMembers,
  type LibraryStatusFilter,
  type LibraryTypeFilter,
} from '../api/feed'

const STATUS_OPTIONS: LibraryStatusFilter[] = ['WATCHING', 'READING', 'COMPLETED', 'PLAN_TO']

const STATUS_LABELS: Record<LibraryStatusFilter, string> = {
  WATCHING: 'Watching',
  READING: 'Reading',
  COMPLETED: 'Completed',
  PLAN_TO: 'Plan to',
}

/**
 * Read-only view of another member's shelf (spec §2.7) with type/status
 * filters. Writes remain strictly own-account, so there are no edit controls.
 */
export default function UserLibrary() {
  const { id = '' } = useParams<{ id: string }>()
  const [typeFilter, setTypeFilter] = useState<LibraryTypeFilter | ''>('')
  const [statusFilter, setStatusFilter] = useState<LibraryStatusFilter | ''>('')

  const { data: members } = useQuery({ queryKey: ['members'], queryFn: fetchMembers })
  const member = members?.find((m) => String(m.id) === id)

  const {
    data: items,
    isPending,
    isError,
    error,
  } = useQuery({
    queryKey: ['memberLibrary', id, typeFilter, statusFilter],
    queryFn: () =>
      fetchMemberLibrary(id, {
        type: typeFilter === '' ? undefined : typeFilter,
        status: statusFilter === '' ? undefined : statusFilter,
      }),
  })

  if (isError && error instanceof ApiError && error.code === 'user_not_found') {
    return (
      <section>
        <h1>Member shelf</h1>
        <p role="alert">No such member.</p>
        <Link to="/feed">Back to the feed</Link>
      </section>
    )
  }

  return (
    <section>
      <h1>{member !== undefined ? `${member.username}'s shelf` : 'Member shelf'}</h1>
      <p style={{ color: '#666' }}>Read-only view of another member&apos;s library.</p>

      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem' }}>
        <label>
          Type{' '}
          <select
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value as LibraryTypeFilter | '')}
          >
            <option value="">All</option>
            <option value="TV">TV</option>
            <option value="BOOK">Books</option>
          </select>
        </label>
        <label>
          Status{' '}
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as LibraryStatusFilter | '')}
          >
            <option value="">All</option>
            {STATUS_OPTIONS.map((s) => (
              <option key={s} value={s}>
                {STATUS_LABELS[s]}
              </option>
            ))}
          </select>
        </label>
      </div>

      {isPending && <p>Loading shelf…</p>}
      {isError && <p role="alert">Could not load this member&apos;s shelf. Try reloading.</p>}

      {items !== undefined && items.length === 0 && <p>Nothing on this shelf (yet).</p>}

      {items !== undefined && items.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0 }}>
          {items.map((item) => (
            <li
              key={item.id}
              style={{ padding: '0.5rem 0', borderBottom: '1px solid #eee' }}
            >
              <strong>{item.title}</strong>{' '}
              <span style={{ color: '#666' }}>
                {item.type === 'TV' ? 'TV' : 'Book'} · {item.status}
                {item.type === 'BOOK' && item.progress > 0 && ` · page ${item.progress}`}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
