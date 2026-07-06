import { ApiError } from '../api/client'
import ShowSearch from '../components/tv/ShowSearch'
import UpNextCard from '../components/tv/UpNextCard'
import { useMarkWatched, useUpNext } from '../hooks/useUpNext'

/**
 * Up Next dashboard: one card per WATCHING show with its earliest
 * aired, unwatched episode and a one-tap watch checkmark. The search/add flow
 * lives on the same page so an empty dashboard leads straight into adding.
 */
export default function UpNext() {
  const upNext = useUpNext()
  const markWatched = useMarkWatched()

  return (
    <section>
      <h1>Up Next</h1>

      {upNext.isPending && <p className="muted">Loading your shows…</p>}
      {upNext.isError && (
        <p role="alert" className="alert">
          {upNext.error instanceof ApiError
            ? upNext.error.message
            : 'Could not load Up Next. Try refreshing.'}
        </p>
      )}

      {upNext.data !== undefined && upNext.data.length === 0 && (
        <p className="empty">
          Nothing to watch right now. Search for a show below to start tracking it, or mark a show
          as Watching from your Library — new episodes appear here after the nightly sync.
        </p>
      )}

      {upNext.data !== undefined && upNext.data.length > 0 && (
        <ul className="list">
          {upNext.data.map((entry) => (
            <UpNextCard
              key={entry.show.id}
              entry={entry}
              onMarkWatched={(episodeId) => markWatched.mutate(episodeId)}
            />
          ))}
        </ul>
      )}

      {markWatched.isError && (
        <p role="alert" className="alert">
          Could not mark the episode watched. Please try again.
        </p>
      )}

      <hr />
      <ShowSearch />
    </section>
  )
}
