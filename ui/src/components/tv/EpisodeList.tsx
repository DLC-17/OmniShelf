import { useState } from 'react'
import type { EpisodeWatchState } from '../../api/tv'
import { useEpisodeActions, useEpisodes } from '../../hooks/useEpisodes'

interface EpisodeListProps {
  showId: number
}

const pad = (n: number) => String(n).padStart(2, '0')
const today = () => new Date().toISOString().slice(0, 10)

/** An episode is markable only once it has an air date that is not in the future. */
function hasAired(ep: EpisodeWatchState): boolean {
  return ep.airDate !== null && ep.airDate <= today()
}

/**
 * Expandable episode picker for one show. Lists every episode grouped by
 * season with its watched state, and lets the user:
 *  - mark an unwatched, aired episode watched — choosing "just this episode"
 *    or "this and everything before it";
 *  - re-stamp a watched episode as rewatched;
 *  - un-watch an episode.
 */
export default function EpisodeList({ showId }: EpisodeListProps) {
  const episodes = useEpisodes(showId, true)
  const actions = useEpisodeActions(showId)
  // Episode id the user is mid-confirming a "watched" choice for.
  const [choosing, setChoosing] = useState<number | null>(null)
  // Seasons are collapsed by default; this holds the expanded season numbers.
  const [openSeasons, setOpenSeasons] = useState<Set<number>>(new Set())

  const toggleSeason = (season: number) =>
    setOpenSeasons((prev) => {
      const next = new Set(prev)
      if (next.has(season)) next.delete(season)
      else next.add(season)
      return next
    })

  const busy =
    actions.watch.isPending ||
    actions.rewatch.isPending ||
    actions.watchThrough.isPending ||
    actions.unwatch.isPending

  if (episodes.isPending) {
    return <p className="muted">Loading episodes…</p>
  }
  if (episodes.isError) {
    return (
      <p role="alert" className="alert">
        Could not load episodes. Try again.
      </p>
    )
  }
  if (episodes.data.length === 0) {
    return <p className="muted">No episodes on record yet — they arrive with the nightly sync.</p>
  }

  // Group into seasons, preserving the server's (season, number) order.
  const seasons = new Map<number, EpisodeWatchState[]>()
  for (const ep of episodes.data) {
    const list = seasons.get(ep.season) ?? []
    list.push(ep)
    seasons.set(ep.season, list)
  }

  return (
    <div className="episodes">
      {[...seasons.entries()].map(([season, eps]) => {
        const open = openSeasons.has(season)
        const watchedCount = eps.filter((e) => e.watched).length
        return (
        <div key={season} className="episode-season">
          <button
            type="button"
            className="episode-season-toggle"
            aria-expanded={open}
            onClick={() => toggleSeason(season)}
          >
            <span className="show-caret" aria-hidden="true">{open ? '▾' : '▸'}</span>
            <span className="episode-season-title">Season {season}</span>
            <span className="badge">
              {watchedCount}/{eps.length}
            </span>
          </button>
          {open && (
          <ul className="episode-rows">
            {eps.map((ep) => {
              const code = `S${pad(ep.season)}E${pad(ep.number)}`
              const aired = hasAired(ep)
              return (
                <li key={ep.id} className={ep.watched ? 'episode-row watched' : 'episode-row'}>
                  <span className="episode-check" aria-hidden="true">
                    {ep.watched ? '✓' : aired ? '○' : '·'}
                  </span>
                  <span className="grow">
                    <span className="episode-code">{code}</span>
                    {ep.title !== '' && <span className="episode-title"> {ep.title}</span>}
                    {!aired && <span className="tag">Unaired</span>}
                  </span>

                  {ep.watched ? (
                    <span className="cluster">
                      <button
                        type="button"
                        className="btn-confirm"
                        disabled={busy}
                        onClick={() => actions.rewatch.mutate(ep.id)}
                      >
                        Rewatched
                      </button>
                      <button
                        type="button"
                        className="btn-ghost"
                        disabled={busy}
                        onClick={() => actions.unwatch.mutate(ep.id)}
                      >
                        Unwatch
                      </button>
                    </span>
                  ) : aired ? (
                    choosing === ep.id ? (
                      <span className="cluster">
                        <button
                          type="button"
                          className="btn-confirm"
                          disabled={busy}
                          onClick={() => {
                            actions.watch.mutate(ep.id)
                            setChoosing(null)
                          }}
                        >
                          Just this
                        </button>
                        <button
                          type="button"
                          className="btn-confirm"
                          disabled={busy}
                          onClick={() => {
                            actions.watchThrough.mutate(ep.id)
                            setChoosing(null)
                          }}
                        >
                          This &amp; all previous
                        </button>
                        <button
                          type="button"
                          className="btn-ghost"
                          disabled={busy}
                          onClick={() => setChoosing(null)}
                        >
                          Cancel
                        </button>
                      </span>
                    ) : (
                      <button
                        type="button"
                        disabled={busy}
                        aria-label={`Mark ${code} watched`}
                        onClick={() => setChoosing(ep.id)}
                      >
                        Mark watched
                      </button>
                    )
                  ) : null}
                </li>
              )
            })}
          </ul>
          )}
        </div>
        )
      })}
    </div>
  )
}
