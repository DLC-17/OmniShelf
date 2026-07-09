import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { addNote, deleteNote, fetchNotes } from '../api/bookNotes'

/** Per-item query key so each book's journal caches independently. */
export const NOTES_KEY = ['notes'] as const

export function useNotes(itemId: number, enabled = true) {
  return useQuery({
    queryKey: [...NOTES_KEY, itemId] as const,
    queryFn: () => fetchNotes(itemId),
    enabled,
  })
}

export function useAddNote(itemId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: string) => addNote(itemId, body),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: [...NOTES_KEY, itemId] })
    },
  })
}

export function useDeleteNote(itemId: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (noteId: number) => deleteNote(itemId, noteId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: [...NOTES_KEY, itemId] })
    },
  })
}
