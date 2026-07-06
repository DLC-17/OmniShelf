import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchMembers } from '../../api/feed'

/**
 * Instance members with tracked-item counts; each links to that member's
 * read-only shelf at /users/:id.
 */
export default function MembersList() {
  const { data: members, isPending, isError } = useQuery({
    queryKey: ['members'],
    queryFn: fetchMembers,
  })

  if (isPending) {
    return <p className="muted">Loading members…</p>
  }
  if (isError) {
    return <p role="alert" className="alert">Could not load members.</p>
  }
  if (members.length === 0) {
    return <p className="muted">No members yet.</p>
  }

  return (
    <ul className="divided">
      {members.map((m) => (
        <li key={m.id}>
          <Link to={`/users/${m.id}`}>{m.username}</Link>{' '}
          <span className="muted">
            ({m.counts.tv} shows, {m.counts.books} books)
          </span>
        </li>
      ))}
    </ul>
  )
}
