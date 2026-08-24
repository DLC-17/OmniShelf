import { useEffect, useRef, useState } from 'react'
import type { ChangeEvent, DragEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../../api/client'
import { addCard, scanCard } from '../../api/cards'
import type { Card, CardGame } from '../../api/cards'
import { CARD_OWNERSHIP, updateOwnership } from '../../api/library'
import { LIBRARY_KEY } from '../../hooks/useLibrary'
import { formatUsd } from '../../lib/currency'
import { fileToJpegBlob } from '../../lib/cardImage'

const QUEUE_STORAGE_KEY = 'omnishelf_card_scan_queue'

export type ScanItemStatus =
  | 'pending'
  | 'compressing'
  | 'identifying'
  | 'ready'
  | 'error'
  | 'added'
  | 'exists'

export interface CardQueueItem {
  id: string
  fileName: string
  status: ScanItemStatus
  card?: Card
  error?: string
  finishes: string[]
  notice?: string
  createdAt: number
}

const GAME_LABELS: Record<CardGame, string> = {
  YUGIOH: 'Yu-Gi-Oh!',
  POKEMON: 'Pokémon',
}

/** Human-readable explanation for each identify miss the backend reports. */
function scanErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'no_text_detected') {
      return 'No text detected — try better lighting or a closer, straight-on shot.'
    }
    if (err.code === 'unsupported_card') {
      return "Couldn't recognize this card type or set code. Try a sharper, straight-on photo."
    }
    if (err.code === 'card_not_found') {
      const setCode = typeof err.details?.setCode === 'string' ? err.details.setCode : ''
      return setCode !== ''
        ? `No card found for set code ${setCode}. Try another photo.`
        : 'No matching card found in catalog. Try another photo.'
    }
    return err.message
  }
  if (err instanceof Error) {
    return err.message
  }
  return 'Something went wrong. Please try again.'
}

function loadPersistedQueue(): CardQueueItem[] {
  try {
    const raw = localStorage.getItem(QUEUE_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as CardQueueItem[]
    if (!Array.isArray(parsed)) return []
    // Clean up any items that were left mid-flight when the tab was previously closed
    return parsed.map((item) => {
      if (item.status === 'pending' || item.status === 'compressing' || item.status === 'identifying') {
        return {
          ...item,
          status: 'error',
          error: 'Scan was interrupted before completing. Please re-upload.',
        }
      }
      return item
    })
  } catch {
    return []
  }
}

function savePersistedQueue(queue: CardQueueItem[]) {
  try {
    localStorage.setItem(QUEUE_STORAGE_KEY, JSON.stringify(queue))
  } catch {
    // Ignore storage quota errors
  }
}

export default function CardScan() {
  const queryClient = useQueryClient()
  const [queue, setQueue] = useState<CardQueueItem[]>(() => loadPersistedQueue())
  const [submittingIds, setSubmittingIds] = useState<Set<string>>(new Set())
  const [bulkAdding, setBulkAdding] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Sync to localStorage whenever queue changes
  useEffect(() => {
    savePersistedQueue(queue)
  }, [queue])

  const updateItem = (id: string, patch: Partial<CardQueueItem>) => {
    setQueue((prev) =>
      prev.map((item) => (item.id === id ? { ...item, ...patch } : item)),
    )
  }

  const removeItem = (id: string) => {
    setQueue((prev) => prev.filter((item) => item.id !== id))
  }

  const clearCompleted = () => {
    setQueue((prev) => prev.filter((item) => item.status !== 'added' && item.status !== 'exists'))
  }

  const clearAll = () => {
    setQueue([])
  }

  const toggleFinish = (itemId: string, finish: string) => {
    setQueue((prev) =>
      prev.map((item) => {
        if (item.id !== itemId) return item
        const exists = item.finishes.includes(finish)
        const finishes = exists
          ? item.finishes.filter((f) => f !== finish)
          : [...item.finishes, finish]
        return { ...item, finishes }
      }),
    )
  }

  const processFile = async (item: CardQueueItem, file: File) => {
    try {
      updateItem(item.id, { status: 'compressing' })
      const jpegBlob = await fileToJpegBlob(file)

      updateItem(item.id, { status: 'identifying' })
      const card = await scanCard(jpegBlob)

      updateItem(item.id, {
        status: 'ready',
        card,
        error: undefined,
      })
    } catch (err) {
      updateItem(item.id, {
        status: 'error',
        error: scanErrorMessage(err),
      })
    }
  }

  const handleFiles = (files: FileList | File[]) => {
    if (!files || files.length === 0) return

    const newItems: { item: CardQueueItem; file: File }[] = []
    const queueAdditions: CardQueueItem[] = []

    Array.from(files).forEach((file) => {
      const id = `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
      const queueItem: CardQueueItem = {
        id,
        fileName: file.name || 'Card photo',
        status: 'pending',
        finishes: [],
        createdAt: Date.now(),
      }
      newItems.push({ item: queueItem, file })
      queueAdditions.push(queueItem)
    })

    setQueue((prev) => [...queueAdditions, ...prev])

    // Process each upload asynchronously in parallel
    newItems.forEach(({ item, file }) => {
      void processFile(item, file)
    })
  }

  const handleFileInputChange = (e: ChangeEvent<HTMLInputElement>) => {
    if (e.target.files) {
      handleFiles(e.target.files)
    }
    e.target.value = '' // Allow re-uploading the same file if desired
  }

  const handleDragOver = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setIsDragging(true)
  }

  const handleDragLeave = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setIsDragging(false)
  }

  const handleDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setIsDragging(false)
    if (e.dataTransfer.files) {
      handleFiles(e.dataTransfer.files)
    }
  }

  const handleAddSingle = async (item: CardQueueItem) => {
    if (!item.card || item.status !== 'ready') return

    setSubmittingIds((prev) => new Set(prev).add(item.id))
    try {
      const { item: trackingItem } = await addCard(item.card)
      if (item.finishes.length > 0) {
        try {
          await updateOwnership(trackingItem.id, item.finishes)
        } catch {
          updateItem(item.id, {
            status: 'added',
            notice: 'Added, but saving finish failed — you can set it in the library.',
          })
          await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
          return
        }
      }
      await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
      updateItem(item.id, { status: 'added', notice: 'Added to your shelf as “Owned”.' })
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        updateItem(item.id, { status: 'exists', notice: 'This card is already on your shelf.' })
      } else {
        updateItem(item.id, {
          error: err instanceof ApiError ? err.message : 'Failed to add card to shelf.',
        })
      }
    } finally {
      setSubmittingIds((prev) => {
        const next = new Set(prev)
        next.delete(item.id)
        return next
      })
    }
  }

  const handleAddAllReady = async () => {
    const readyItems = queue.filter((i) => i.status === 'ready' && i.card)
    if (readyItems.length === 0 || bulkAdding) return

    setBulkAdding(true)
    for (const item of readyItems) {
      if (!item.card) continue
      try {
        const { item: trackingItem } = await addCard(item.card)
        if (item.finishes.length > 0) {
          try {
            await updateOwnership(trackingItem.id, item.finishes)
          } catch {
            // best-effort
          }
        }
        updateItem(item.id, { status: 'added', notice: 'Added to shelf.' })
      } catch (err) {
        if (err instanceof ApiError && err.status === 409) {
          updateItem(item.id, { status: 'exists', notice: 'Already on shelf.' })
        } else {
          updateItem(item.id, {
            error: err instanceof ApiError ? err.message : 'Failed to add.',
          })
        }
      }
    }
    await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
    setBulkAdding(false)
  }

  const readyCount = queue.filter((i) => i.status === 'ready').length
  const processingCount = queue.filter(
    (i) => i.status === 'pending' || i.status === 'compressing' || i.status === 'identifying',
  ).length
  const addedCount = queue.filter((i) => i.status === 'added' || i.status === 'exists').length
  const errorCount = queue.filter((i) => i.status === 'error').length

  return (
    <div className="stack" style={{ gap: '1.25rem' }}>
      <div className="callout">
        <strong>Batch Trading Card Scanner</strong>
        <p style={{ margin: '0.35rem 0 0' }}>
          Upload single or multiple card photos (Yu-Gi-Oh! or Pokémon). Scans run asynchronously in
          the background and remain saved here so you can confirm and add them at any time.
        </p>
      </div>

      {/* Drag & drop upload zone */}
      <div
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        style={{
          border: `2px dashed ${isDragging ? 'var(--accent, #6366f1)' : 'var(--border)'}`,
          borderRadius: 'var(--radius)',
          padding: '1.5rem',
          textAlign: 'center',
          background: isDragging ? 'var(--surface-alt)' : 'var(--surface)',
          transition: 'all 0.15s ease',
          cursor: 'pointer',
        }}
        onClick={() => fileInputRef.current?.click()}
      >
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          multiple
          style={{ display: 'none' }}
          aria-label="Upload card photos"
          onChange={handleFileInputChange}
        />
        <div className="stack" style={{ alignItems: 'center', gap: '0.5rem' }}>
          <span style={{ fontSize: '1.75rem' }} aria-hidden="true">
            📷
          </span>
          <strong>Select or Drop Card Photos</strong>
          <span className="muted" style={{ fontSize: '0.875rem' }}>
            Choose one or multiple photos to scan at once
          </span>
          <button
            type="button"
            className="btn-primary"
            style={{ marginTop: '0.25rem' }}
            onClick={(e) => {
              e.stopPropagation()
              fileInputRef.current?.click()
            }}
          >
            Choose Files
          </button>
        </div>
      </div>

      {/* Summary and Batch Action Bar */}
      {queue.length > 0 && (
        <div
          className="card"
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            flexWrap: 'wrap',
            gap: '0.75rem',
            padding: '0.75rem 1rem',
          }}
        >
          <div className="cluster" style={{ gap: '0.6rem' }}>
            {readyCount > 0 && (
              <span className="badge badge-ok">
                <strong>{readyCount}</strong> ready to confirm
              </span>
            )}
            {processingCount > 0 && (
              <span className="badge">
                <strong>{processingCount}</strong> scanning…
              </span>
            )}
            {addedCount > 0 && (
              <span className="badge">
                <strong>{addedCount}</strong> shelved
              </span>
            )}
            {errorCount > 0 && (
              <span className="badge" style={{ color: 'var(--danger, #ef4444)' }}>
                <strong>{errorCount}</strong> failed
              </span>
            )}
          </div>

          <div className="cluster" style={{ gap: '0.5rem' }}>
            {readyCount > 0 && (
              <button
                type="button"
                className="btn-confirm"
                onClick={handleAddAllReady}
                disabled={bulkAdding}
              >
                {bulkAdding ? 'Adding all…' : `Confirm All Ready (${readyCount})`}
              </button>
            )}
            {addedCount > 0 && (
              <button type="button" className="btn-ghost" onClick={clearCompleted}>
                Clear Shelved
              </button>
            )}
            <button type="button" className="btn-ghost" onClick={clearAll}>
              Clear All
            </button>
          </div>
        </div>
      )}

      {/* Scanned Items Queue */}
      {queue.length > 0 && (
        <div className="stack" style={{ gap: '0.75rem' }}>
          {queue.map((item) => {
            const isSubmitting = submittingIds.has(item.id) || bulkAdding
            const card = item.card

            // Progress / Pending State
            if (
              item.status === 'pending' ||
              item.status === 'compressing' ||
              item.status === 'identifying'
            ) {
              return (
                <div
                  key={item.id}
                  className="card card-row"
                  style={{ alignItems: 'center', padding: '0.85rem 1rem' }}
                >
                  <div
                    aria-hidden="true"
                    className="poster placeholder"
                    style={{ width: 48, height: 68, fontSize: '0.65rem' }}
                  >
                    …
                  </div>
                  <div className="grow">
                    <strong>{item.fileName}</strong>
                    <p className="muted" style={{ margin: '0.2rem 0 0', fontSize: '0.85rem' }}>
                      {item.status === 'compressing'
                        ? 'Optimizing image…'
                        : item.status === 'identifying'
                          ? 'Recognizing card with Vision OCR…'
                          : 'Queued for scan…'}
                    </p>
                  </div>
                  <button
                    type="button"
                    className="btn-ghost"
                    style={{ padding: '0.25rem 0.6rem', fontSize: '0.85rem' }}
                    onClick={() => removeItem(item.id)}
                  >
                    Cancel
                  </button>
                </div>
              )
            }

            // Error State
            if (item.status === 'error') {
              return (
                <div
                  key={item.id}
                  className="card card-row"
                  style={{
                    alignItems: 'center',
                    padding: '0.85rem 1rem',
                    borderColor: 'var(--danger-border, #fca5a5)',
                  }}
                >
                  <div
                    aria-hidden="true"
                    className="poster placeholder"
                    style={{ width: 48, height: 68, fontSize: '0.65rem', color: 'var(--danger, #ef4444)' }}
                  >
                    Miss
                  </div>
                  <div className="grow">
                    <strong>{item.fileName}</strong>
                    <p className="alert" style={{ margin: '0.2rem 0 0', fontSize: '0.85rem' }}>
                      {item.error || 'Identification failed.'}
                    </p>
                  </div>
                  <button
                    type="button"
                    className="btn-ghost"
                    style={{ padding: '0.25rem 0.6rem', fontSize: '0.85rem' }}
                    onClick={() => removeItem(item.id)}
                  >
                    Dismiss
                  </button>
                </div>
              )
            }

            // Shelved / Exists State
            if (item.status === 'added' || item.status === 'exists') {
              const artSrc =
                card?.coverPath && card.coverPath !== ''
                  ? card.coverPath.startsWith('/')
                    ? card.coverPath
                    : `/images/${card.coverPath}`
                  : null

              return (
                <div
                  key={item.id}
                  className="card card-row"
                  style={{ alignItems: 'center', padding: '0.85rem 1rem' }}
                >
                  {artSrc !== null ? (
                    <img
                      src={artSrc}
                      alt={card?.name ?? item.fileName}
                      width={48}
                      height={68}
                      className="poster"
                      style={{ objectFit: 'cover' }}
                    />
                  ) : (
                    <div
                      aria-hidden="true"
                      className="poster placeholder"
                      style={{ width: 48, height: 68, fontSize: '0.65rem' }}
                    >
                      Card
                    </div>
                  )}
                  <div className="grow">
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                      <strong>{card?.name ?? item.fileName}</strong>
                      {card && <span className="badge">{GAME_LABELS[card.game]}</span>}
                    </div>
                    {card?.setCode && (
                      <p className="meta" style={{ margin: '0.2rem 0 0', fontSize: '0.85rem' }}>
                        {card.setName} ({card.setCode})
                      </p>
                    )}
                    <p className="notice" style={{ margin: '0.2rem 0 0', fontSize: '0.85rem' }}>
                      {item.notice ?? (item.status === 'added' ? 'Added to shelf' : 'Already on shelf')}
                    </p>
                  </div>
                  <button
                    type="button"
                    className="btn-ghost"
                    style={{ padding: '0.25rem 0.6rem', fontSize: '0.85rem' }}
                    onClick={() => removeItem(item.id)}
                  >
                    Dismiss
                  </button>
                </div>
              )
            }

            // Ready for Confirmation State
            if (card && item.status === 'ready') {
              const artSrc =
                card.coverPath !== ''
                  ? card.coverPath.startsWith('/')
                    ? card.coverPath
                    : `/images/${card.coverPath}`
                  : null

              const typeLine = [card.cardType, card.race].filter(Boolean).join(' · ')
              const setLine = [card.setName, card.setCode].filter(Boolean).join(' · ')

              return (
                <div
                  key={item.id}
                  className="card"
                  style={{
                    padding: '1rem',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius)',
                  }}
                >
                  <div className="card-row" style={{ alignItems: 'flex-start' }}>
                    {artSrc !== null ? (
                      <img
                        src={artSrc}
                        alt={card.name}
                        width={72}
                        height={104}
                        className="poster"
                        style={{ objectFit: 'cover' }}
                      />
                    ) : (
                      <div
                        aria-hidden="true"
                        className="poster placeholder"
                        style={{ width: 72, height: 104, fontSize: '0.7rem' }}
                      >
                        No art
                      </div>
                    )}
                    <div className="grow">
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
                        <strong style={{ fontSize: '1.05rem' }}>{card.name}</strong>
                        <span className="badge">{GAME_LABELS[card.game]}</span>
                        {card.price > 0 && (
                          <strong style={{ color: 'var(--text)', marginLeft: 'auto' }}>
                            {formatUsd(card.price)}
                          </strong>
                        )}
                      </div>

                      {typeLine !== '' && (
                        <p className="meta" style={{ margin: '0.2rem 0 0', fontSize: '0.85rem' }}>
                          {typeLine}
                        </p>
                      )}
                      {setLine !== '' && (
                        <p className="meta" style={{ margin: '0.2rem 0 0', fontSize: '0.85rem' }}>
                          {setLine}
                        </p>
                      )}
                      {card.artist !== '' && (
                        <p className="meta" style={{ margin: '0.2rem 0 0', fontSize: '0.85rem' }}>
                          Illus. {card.artist}
                        </p>
                      )}

                      {/* Finish Selection */}
                      <div
                        className="cluster"
                        style={{ marginTop: '0.6rem', gap: '0.75rem', alignItems: 'center' }}
                      >
                        <span className="muted" style={{ fontSize: '0.85rem' }}>
                          Finish:
                        </span>
                        {CARD_OWNERSHIP.map((finish) => (
                          <label
                            key={finish}
                            className="cluster"
                            style={{ gap: '0.3rem', fontSize: '0.85rem' }}
                          >
                            <input
                              type="checkbox"
                              checked={item.finishes.includes(finish)}
                              disabled={isSubmitting}
                              onChange={() => toggleFinish(item.id, finish)}
                            />
                            {finish}
                          </label>
                        ))}
                      </div>

                      {item.error && (
                        <p role="alert" className="alert" style={{ margin: '0.5rem 0 0', fontSize: '0.85rem' }}>
                          {item.error}
                        </p>
                      )}

                      {/* Actions */}
                      <div
                        className="cluster"
                        style={{ marginTop: '0.75rem', gap: '0.5rem', justifyContent: 'flex-start' }}
                      >
                        <button
                          type="button"
                          className="btn-confirm"
                          style={{ padding: '0.35rem 0.85rem', fontSize: '0.875rem' }}
                          onClick={() => handleAddSingle(item)}
                          disabled={isSubmitting}
                        >
                          {isSubmitting ? 'Adding…' : 'Confirm & Add to Shelf'}
                        </button>
                        <button
                          type="button"
                          className="btn-ghost"
                          style={{ padding: '0.35rem 0.75rem', fontSize: '0.875rem' }}
                          onClick={() => removeItem(item.id)}
                          disabled={isSubmitting}
                        >
                          Remove
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              )
            }

            return null
          })}
        </div>
      )}
    </div>
  )
}
