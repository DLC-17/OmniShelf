import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchMembers } from '../../api/feed'

/**
 * Instance members with tracked-item counts; each links to that member's
 * read-only shelf at /users/:id (spec §2.7).
 */
export default function MembersList() {
  const { data: members, isPending, isError } = useQuery({
    queryKey: ['members'],
    queryFn: fetchMembers,
  })

  if (isPending) {
    return <p>Loading members…</p>
  }
  if (isError) {
    return <p role="alert">Could not load members.</p>
  }
  if (members.length === 0) {
    return <p>No members yet.</p>
  }

  return (
    <ul style={{ listStyle: 'none', padding: 0 }}>
      {members.map((m) => (
        <li key={m.id} style={{ padding: '0.25rem 0' }}>
          <Link to={`/users/${m.id}`}>{m.username}</Link>{' '}
          <span style={{ color: '#666' }}>
            ({m.counts.tv} shows, {m.counts.books} books)
          </span>
        </li>
      ))}
    </ul>
  )
}
