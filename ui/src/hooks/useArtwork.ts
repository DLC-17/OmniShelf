import { useMutation, useQueryClient } from '@tanstack/react-query'
import { refreshArtwork, uploadArtwork } from '../api/artwork'
import { LIBRARY_KEY } from './useLibrary'
import { UP_NEXT_KEY } from './useUpNext'
import { UPCOMING_KEY } from './useUpcoming'

/** Invalidate every view that renders a cover so the new art propagates. */
function invalidateArtwork(queryClient: ReturnType<typeof useQueryClient>) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: LIBRARY_KEY }),
    queryClient.invalidateQueries({ queryKey: UP_NEXT_KEY }),
    queryClient.invalidateQueries({ queryKey: UPCOMING_KEY }),
  ])
}

/** Re-pull cover art from the upstream source (TMDB/IGDB/OpenLibrary). */
export function useRefreshArtwork() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (itemId: number) => refreshArtwork(itemId),
    onSuccess: () => invalidateArtwork(queryClient),
  })
}

/** Upload a custom cover image for a tracked item. */
export function useUploadArtwork() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ itemId, file }: { itemId: number; file: File }) => uploadArtwork(itemId, file),
    onSuccess: () => invalidateArtwork(queryClient),
  })
}
