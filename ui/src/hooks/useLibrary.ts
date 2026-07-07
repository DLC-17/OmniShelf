import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deleteItem, fetchLibrary, updateItem } from '../api/library'
import type { LibraryFilters, UpdateItemPatch } from '../api/library'
import { UP_NEXT_KEY } from './useUpNext'

export const LIBRARY_KEY = ['library'] as const

export function useLibrary(filters: LibraryFilters, enabled = true) {
  return useQuery({
    // Filters are part of the key so each type/status combination caches independently.
    queryKey: [...LIBRARY_KEY, filters.type ?? '', filters.status ?? ''] as const,
    queryFn: () => fetchLibrary(filters),
    enabled,
  })
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
