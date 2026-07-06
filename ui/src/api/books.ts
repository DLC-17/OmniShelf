import { request } from './client'

/** A Book payload from POST /api/books/scan (matches api.bookResponse). */
export interface Book {
  id: number
  isbn13: string
  title: string
  authors: string
  coverPath: string
  pageCount: number
}

/** A TrackingItem payload from POST /api/books/track (matches api.itemResponse). */
export interface TrackingItem {
  id: number
  type: string
  externalId: string
  title: string
  status: string
  progress: number
  updatedAt: string
}

/** Tracking statuses a book may take (spec §2.5 step 4). */
export type BookStatus = 'READING' | 'PLAN_TO' | 'COMPLETED'

/**
 * Scan an ISBN-13. A 404 ({error:"book_not_found", isbn}) surfaces as an
 * ApiError with status 404 so the caller can offer the manual-entry form
 * pre-filled with the ISBN (E4).
 */
export function scanBook(isbn: string): Promise<Book> {
  return request<Book>('/api/books/scan', { method: 'POST', body: { isbn } })
}

/**
 * Track a scanned book. A 409 ({error:"already_tracked"}) surfaces as an
 * ApiError with status 409 so the caller can report it without treating it as
 * a hard failure (E16).
 */
export function trackBook(bookId: number, status: BookStatus): Promise<TrackingItem> {
  return request<TrackingItem>('/api/books/track', {
    method: 'POST',
    body: { bookId, status },
  })
}
