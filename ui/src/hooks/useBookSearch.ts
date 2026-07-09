import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listEditions, scanBook, searchBooks, trackBook } from '../api/books'
import type { BookStatus } from '../api/books'
import { LIBRARY_KEY } from './useLibrary'

/** OpenLibrary title search; disabled until the user submits a non-empty query. */
export function useBookSearch(query: string) {
  return useQuery({
    queryKey: ['books', 'search', query] as const,
    queryFn: () => searchBooks(query),
    enabled: query !== '',
  })
}

/** Editions of a chosen work (the ISBN picker); disabled until a work is picked. */
export function useBookEditions(workKey: string | null) {
  return useQuery({
    queryKey: ['books', 'editions', workKey] as const,
    queryFn: () => listEditions(workKey as string),
    enabled: workKey !== null,
  })
}

/**
 * Adds a book by a chosen edition's ISBN: scan (upsert the shared Book row) then
 * track it. A fresh add creates a PLAN_TO watchlist item, so the library shelf
 * is refreshed on success.
 */
export function useAddBookByIsbn() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (isbn: string) => {
      const book = await scanBook(isbn)
      const status: BookStatus = 'PLAN_TO'
      return trackBook(book.id, status)
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
    },
  })
}
