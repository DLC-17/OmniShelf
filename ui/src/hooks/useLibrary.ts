import { useEffect, useMemo } from 'react'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deleteItem, fetchLibrary, fetchLibraryItem, updateItem, updateOwnership } from '../api/library'
import type { LibraryFilters, LibraryItem, LibraryPage, UpdateItemPatch } from '../api/library'
import { UP_NEXT_KEY } from './useUpNext'

export const LIBRARY_KEY = ['library'] as const
export const STALE_TIME = 5 * 60 * 1000 // 5 minutes

export function useLibrary(filters: LibraryFilters, enabled = true) {
  return useQuery({
    // Filters are part of the key so each combination caches independently.
    queryKey: [
      ...LIBRARY_KEY,
      filters.type ?? '',
      filters.status ?? '',
      filters.search ?? '',
      filters.rating ?? '',
      filters.tag ?? '',
      filters.location ?? '',
    ] as const,
    queryFn: async () => {
      const page = await fetchLibrary(filters)
      return page.items
    },
    staleTime: STALE_TIME,
    enabled,
  })
}

export function useInfiniteLibrary(filters: LibraryFilters, enabled = true) {
  return useInfiniteQuery<LibraryPage>({
    queryKey: [
      ...LIBRARY_KEY,
      'infinite',
      filters.type ?? '',
      filters.status ?? '',
      filters.search ?? '',
      filters.rating ?? '',
      filters.tag ?? '',
      filters.location ?? '',
    ] as const,
    queryFn: ({ pageParam }) =>
      fetchLibrary({ ...filters, after: (pageParam as number) || undefined }),
    initialPageParam: 0,
    getNextPageParam: (lastPage) => {
      if (!lastPage.hasMore || lastPage.items.length === 0) return undefined
      return lastPage.items[lastPage.items.length - 1].id
    },
    staleTime: STALE_TIME,
    enabled,
  })
}

export function useLibraryItem(id: number, enabled = true) {
  return useQuery<LibraryItem>({
    queryKey: [...LIBRARY_KEY, 'item', id],
    queryFn: () => fetchLibraryItem(id),
    staleTime: STALE_TIME,
    enabled: enabled && id > 0,
  })
}

/**
 * Fetches ALL pages of library items for the given filters by auto-scrolling
 * through useInfiniteLibrary. Use this when you need the complete dataset
 * (e.g. card valuations, stats aggregations) rather than incremental loading.
 */
export function useAllLibraryItems(filters: LibraryFilters) {
  const query = useInfiniteLibrary(filters)

  // Auto-fetch remaining pages as they become available.
  useEffect(() => {
    if (query.hasNextPage && !query.isFetchingNextPage) {
      query.fetchNextPage()
    }
  }, [query.hasNextPage, query.isFetchingNextPage, query.fetchNextPage])

  const items = useMemo<LibraryItem[]>(
    () => query.data?.pages.flatMap((p) => p.items) ?? [],
    [query.data],
  )

  return {
    data: items,
    isLoading: query.isLoading,
    isPending: query.isPending,
    isError: query.isError,
    error: query.error,
    isFullyLoaded: !query.isLoading && !query.hasNextPage,
  }
}

export function useUpdateItem() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ id, patch }: { id: number; patch: UpdateItemPatch }) => updateItem(id, patch),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
    },
  })
}

export function useUpdateOwnership() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ id, formats }: { id: number; formats: string[] }) => updateOwnership(id, formats),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
    },
  })
}

export function useDeleteItem() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: number) => deleteItem(id),
    onSuccess: async () => {
      // Untracking a WATCHING show also removes its Up Next card server-side.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: LIBRARY_KEY }),
        queryClient.invalidateQueries({ queryKey: UP_NEXT_KEY }),
      ])
    },
  })
}
