import { useState } from 'react'
import { ApiError } from '../api/client'
import type { UpNextFilter } from '../api/tv'
import ShowSearch from '../components/tv/ShowSearch'
import UpNextCard from '../components/tv/UpNextCard'
import { useMarkWatched, useUpNext } from '../hooks/useUpNext'

const TABS: { value: UpNextFilter; label: string }[] = [
  { value: 'recent', label: 'Recently watched' },
  { value: 'stale', label: "Haven't watched in a while" },
  { value: 'unstarted', label: "Haven't started" },
]

const EMPTY_MESSAGE: Record<UpNextFilter, string> = {
  recent:
    'Nothing watched in the last two weeks. Check "Haven’t watched in a while" to pick something back up, or add a show below.',
  stale: 'Nothing gone cold — everything you’re watching is recent.',
  unstarted: 'No un-started shows. Add one below, or import your history from Settings.',
}

/**
 * Up Next dashboard: one card per WATCHING show with its earliest aired,
 * unwatched episode and a one-tap watch checkmark, bucketed by how recently
 * you last watched it. The search/add flow lives on the same page.
 */
export default function UpNext() {
  const [filter, setFilter] = useState<UpNextFilter>('recent')
  const upNext = useUpNext(filter)
  const markWatched = useMarkWatched()

  return (
    <section>
      <h1>Up Next</h1>
      <div className="tabs" role="tablist" aria-label="Up Next filter">
        {TABS.map((tab) => (
          <button
            key={tab.value}
            type="button"
            role="tab"
            aria-selected={filter === tab.value}
            className={filter === tab.value ? 'tab active' : 'tab'}
            onClick={() => setFilter(tab.value)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {upNext.isPending && <p className="muted">Loading your shows…</p>}
      {upNext.isError && (
        <p role="alert" className="alert">
          {upNext.error instanceof ApiError
            ? upNext.error.message
            : 'Could not load Up Next. Try refreshing.'}
        </p>
      )}

      {upNext.data !== undefined && upNext.data.length === 0 && (
        <p className="empty">{EMPTY_MESSAGE[filter]}</p>
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
