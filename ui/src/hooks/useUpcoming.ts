import { useQuery } from '@tanstack/react-query'
import { fetchUpcoming } from '../api/tv'
import type { UpcomingByType } from '../api/tv'

/** Query key for the cross-media Upcoming board. */
export const UPCOMING_KEY = ['upcoming'] as const

/**
 * Loads the Upcoming board: future TV episodes and movie releases the user
 * follows, grouped by media type. Games and Books come back as empty arrays
 * (no release date is cached for scan-based media).
 */
export function useUpcoming() {
  return useQuery<UpcomingByType>({
    queryKey: UPCOMING_KEY,
    queryFn: fetchUpcoming,
  })
}
