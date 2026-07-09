import { request } from './client'

/** One timestamped journal entry a user attached to a tracked book. */
export interface BookNote {
  id: number
  body: string
  createdAt: string
  updatedAt: string
}

/** List a book item's notes, newest first. */
export function fetchNotes(itemId: number): Promise<BookNote[]> {
  return request<BookNote[]>(`/api/items/${itemId}/notes`)
}

/** Append a journal entry to a book item. */
export function addNote(itemId: number, body: string): Promise<BookNote> {
  return request<BookNote>(`/api/items/${itemId}/notes`, { method: 'POST', body: { body } })
}

/** Delete one journal entry from a book item. */
export function deleteNote(itemId: number, noteId: number): Promise<void> {
  return request<void>(`/api/items/${itemId}/notes/${noteId}`, { method: 'DELETE' })
}
