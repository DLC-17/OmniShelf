import { useInfiniteQuery } from '@tanstack/react-query'
import { fetchFeed, type FeedEntry } from '../api/feed'
import MembersList from '../components/feed/MembersList'

/**
 * Stable list key: the backend's total order (timestamp, source, row) means
 * an entry's identity is captured by its rendered fields.
 */
function entryKey(e: FeedEntry): string {
  return `${e.timestamp}|${e.user.id}|${e.media.type}|${e.media.id}|${e.action}`
}

function formatTimestamp(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

/**
 * Household activity feed (spec §2.7): reverse-chronological entries across
 * all users, paginated with the server's opaque `nextBefore` cursor. The
 * cursor is passed back verbatim, so appended pages never duplicate or skip
 * entries.
 */
export default function Feed() {
  const {
    data,
    isPending,
    isError,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: ['feed'],
    queryFn: ({ pageParam }) => fetchFeed(pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage) => lastPage.nextBefore,
  })

  const entries = data?.pages.flatMap((p) => p.entries) ?? []

  return (
    <section>
      <h1>Feed</h1>

      {isPending && <p>Loading activity…</p>}
      {isError && <p role="alert">Could not load the activity feed. Try reloading.</p>}

      {!isPending && !isError && entries.length === 0 && (
        <p>
          Nothing here yet. Activity shows up as household members watch episodes, add shows and
          books, or finish reading — start by adding something to your library.
        </p>
      )}

      {entries.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0 }}>
          {entries.map((e) => (
            <li key={entryKey(e)} style={{ padding: '0.5rem 0', borderBottom: '1px solid #eee' }}>
              <strong>{e.user.username}</strong> {e.action}
              <div style={{ fontSize: '0.85rem', color: '#666' }}>
                {formatTimestamp(e.timestamp)}
              </div>
            </li>
          ))}
        </ul>
      )}

      {hasNextPage && (
        <button type="button" onClick={() => void fetchNextPage()} disabled={isFetchingNextPage}>
          {isFetchingNextPage ? 'Loading…' : 'Load more'}
        </button>
      )}

      <h2>Members</h2>
      <MembersList />
    </section>
  )
}
