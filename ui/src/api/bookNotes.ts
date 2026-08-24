import { request } from './client'

/** One timestamped journal entry or emotional reaction a user attached to a tracked media item. */
export interface Note {
  id: number
  body: string
  sentiment?: string
  createdAt: string
  updatedAt: string
}

/** Backward compatibility alias for BookNote. */
export type BookNote = Note

/** List an item's notes/journal entries, newest first. */
export function fetchNotes(itemId: number): Promise<Note[]> {
  return request<Note[]>(`/api/items/${itemId}/diary`).catch(() =>
    request<Note[]>(`/api/items/${itemId}/notes`),
  )
}

/** Append a journal entry or sentiment note to an item. */
export function addNote(itemId: number, body: string, sentiment?: string): Promise<Note> {
  const payload: { body: string; sentiment?: string } = { body }
  if (sentiment) payload.sentiment = sentiment

  return request<Note>(`/api/items/${itemId}/diary`, {
    method: 'POST',
    body: payload,
  }).catch(() =>
    request<Note>(`/api/items/${itemId}/notes`, {
      method: 'POST',
      body: { body },
    }),
  )
}

/** Delete one journal entry from an item. */
export function deleteNote(itemId: number, noteId: number): Promise<void> {
  return request<void>(`/api/diary/${noteId}`, { method: 'DELETE' }).catch(() =>
    request<void>(`/api/items/${itemId}/notes/${noteId}`, { method: 'DELETE' }),
  )
}

