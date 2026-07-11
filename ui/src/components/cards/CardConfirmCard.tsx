import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../../api/client'
import { addCard } from '../../api/cards'
import type { Card, CardGame } from '../../api/cards'
import { CARD_OWNERSHIP, updateOwnership } from '../../api/library'
import { LIBRARY_KEY } from '../../hooks/useLibrary'
import { formatUsd } from '../../lib/currency'

interface CardConfirmCardProps {
  card: Card
  /** Reset back to the capture view to scan another card. */
  onDone: () => void
}

const GAME_LABELS: Record<CardGame, string> = {
  YUGIOH: 'Yu-Gi-Oh!',
  POKEMON: 'Pokémon',
}

/**
 * Confirm card for an identified trading card: art, name, game badge,
 * type/race, set and market price, then POST /api/cards/add (the server
 * defaults the status to OWNED). A 409 already_tracked is reported as an
 * informational notice rather than a hard error; a successful add invalidates
 * the library cache so the Cards tab refreshes.
 */
export default function CardConfirmCard({ card, onDone }: CardConfirmCardProps) {
  const queryClient = useQueryClient()
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [added, setAdded] = useState(false)
  // The finish (Holo / Reverse Holo) of the physical copy being shelved; sent
  // as the item's ownership formats right after the add.
  const [finishes, setFinishes] = useState<string[]>([])

  const toggleFinish = (finish: string) =>
    setFinishes((prev) =>
      prev.includes(finish) ? prev.filter((f) => f !== finish) : [...prev, finish],
    )

  const handleAdd = async () => {
    setError(null)
    setNotice(null)
    setSubmitting(true)
    try {
      const { item } = await addCard(card)
      if (finishes.length > 0) {
        try {
          await updateOwnership(item.id, finishes)
        } catch {
          // The card itself is shelved; a failed finish write is recoverable
          // from the library detail, so surface it as a notice, not an error.
          setNotice('Added, but saving the finish failed — set it from the library.')
        }
      }
      await queryClient.invalidateQueries({ queryKey: LIBRARY_KEY })
      setAdded(true)
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setNotice('This card is already on your shelf.')
        setAdded(true)
      } else if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Something went wrong. Please try again.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  // The scan response carries a ready-to-use "/images/..." URL.
  const artSrc =
    card.coverPath === ''
      ? null
      : card.coverPath.startsWith('/')
        ? card.coverPath
        : `/images/${card.coverPath}`

  const typeLine = [card.cardType, card.race].filter((v) => v !== '').join(' · ')
  const setLine = [card.setName, card.setCode].filter((v) => v !== '').join(' · ')

  return (
    <section aria-label="Confirm card" className="card" style={{ maxWidth: '28rem', margin: '0 auto' }}>
      <div className="card-row" style={{ alignItems: 'flex-start' }}>
        {artSrc !== null ? (
          <img src={artSrc} alt={`Art for ${card.name}`} width={96} height={140} className="poster" />
        ) : (
          <div aria-hidden="true" className="poster placeholder" style={{ width: 96, height: 140, fontSize: '0.75rem' }}>
            No art
          </div>
        )}
        <div className="grow">
          <h2>{card.name}</h2>
          <p style={{ margin: 0 }}>
            <span className="badge">{GAME_LABELS[card.game]}</span>
          </p>
          {typeLine !== '' && <p className="meta" style={{ marginTop: '0.25rem' }}>{typeLine}</p>}
          {setLine !== '' && <p className="meta" style={{ marginTop: '0.25rem' }}>{setLine}</p>}
          {card.artist !== '' && <p className="meta" style={{ marginTop: '0.25rem' }}>Illus. {card.artist}</p>}
          <p style={{ marginTop: '0.25rem' }}>
            <strong>{formatUsd(card.price)}</strong>
          </p>
        </div>
      </div>

      {added ? (
        <div className="stack" style={{ marginTop: '1rem' }}>
          <p role="status" className="notice">
            {notice ?? 'Added to your shelf as “Owned”.'}
          </p>
          <div>
            <button type="button" className="btn-ghost" onClick={onDone}>
              Scan another
            </button>
          </div>
        </div>
      ) : (
        <div className="stack" style={{ marginTop: '1rem' }}>
          {error !== null && (
            <p role="alert" className="alert">
              {error}
            </p>
          )}
          <fieldset className="field" style={{ border: 0, padding: 0, margin: 0 }}>
            <legend className="muted">Finish</legend>
            <div className="cluster">
              {CARD_OWNERSHIP.map((finish) => (
                <label key={finish} className="cluster" style={{ gap: '0.35rem' }}>
                  <input
                    type="checkbox"
                    checked={finishes.includes(finish)}
                    disabled={submitting}
                    onChange={() => toggleFinish(finish)}
                  />
                  {finish}
                </label>
              ))}
            </div>
          </fieldset>
          <div className="cluster">
            <button type="button" className="btn-confirm" onClick={handleAdd} disabled={submitting}>
              {submitting ? 'Adding…' : 'Confirm and Add to Shelf'}
            </button>
            <button type="button" className="btn-ghost" onClick={onDone} disabled={submitting}>
              Scan another
            </button>
          </div>
        </div>
      )}
    </section>
  )
}
