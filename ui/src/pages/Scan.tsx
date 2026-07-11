// pages/Scan.tsx
import { useState } from 'react'
import { ApiError } from '../api/client'
import { scanBook } from '../api/books'
import type { Book } from '../api/books'
import { isSecureContext } from '../lib/secureContext'
import { bookScanTarget, gameScanTarget } from '../lib/scanTargets'
import ScannerView from '../components/books/ScannerView'
import BookConfirmCard from '../components/books/BookConfirmCard'
import BulkScanner from '../components/books/BulkScanner'
import ManualIsbnForm from '../components/books/ManualIsbnForm'
// Aliased to prevent naming collision with our GameScan export below
import GameCameraScan from '../components/games/GameScan'
import MusicScan from '../components/music/MusicScan'
import CardScan from '../components/cards/CardScan'

type ScanMode = 'camera' | 'bulk'
type ScanMedia = 'book' | 'game' | 'music' | 'card'

// ─── EXPORTABLE STANDALONE COMPONENTS ───────────────────────────────────────

export function BookScan() {
  return (
    <div className="standalone-bulk-scanner book-scan">
      <BulkScanner target={bookScanTarget} />
    </div>
  )
}

export function GameScan() {
  return (
    <div className="standalone-bulk-scanner game-scan">
      <BulkScanner target={gameScanTarget} />
    </div>
  )
}

// ─── MAIN PAGE COMPONENT ────────────────────────────────────────────────────

export default function Scan() {
  const secure = isSecureContext()

  const [scanMedia, setScanMedia] = useState<ScanMedia>('book')
  const [mode, setMode] = useState<ScanMode>('camera')
  
  const [book, setBook] = useState<Book | null>(null)
  const [looking, setLooking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notFoundIsbn, setNotFoundIsbn] = useState<string | null>(null)
  const [cameraDenied, setCameraDenied] = useState(false)
  const [manualMode, setManualMode] = useState(false)

  const handleIsbn = async (isbn: string) => {
    setError(null)
    setNotFoundIsbn(null)
    setLooking(true)
    try {
      setBook(await scanBook(isbn))
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setNotFoundIsbn(isbn)
      } else if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Something went wrong. Please try again.')
      }
    } finally {
      setLooking(false)
    }
  }

  const reset = () => {
    setBook(null)
    setError(null)
    setNotFoundIsbn(null)
    setCameraDenied(false)
    setManualMode(false)
  }

  if (book !== null) {
    return (
      <section>
        <h1>Scan</h1>
        <BookConfirmCard book={book} onDone={reset} />
      </section>
    )
  }

  const showManual = !secure || cameraDenied || notFoundIsbn !== null || manualMode

  return (
    <section>
      <h1>Scan</h1>

      <div className="tabs" role="tablist" aria-label="Media to scan">
        <button
          type="button"
          role="tab"
          aria-selected={scanMedia === 'book'}
          className={scanMedia === 'book' ? 'tab active' : 'tab'}
          onClick={() => setScanMedia('book')}
        >
          Books
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={scanMedia === 'game'}
          className={scanMedia === 'game' ? 'tab active' : 'tab'}
          onClick={() => setScanMedia('game')}
        >
          Games
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={scanMedia === 'music'}
          className={scanMedia === 'music' ? 'tab active' : 'tab'}
          onClick={() => setScanMedia('music')}
        >
          Music
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={scanMedia === 'card'}
          className={scanMedia === 'card' ? 'tab active' : 'tab'}
          onClick={() => setScanMedia('card')}
        >
          Cards
        </button>
      </div>

      {/* Music, games and cards are self-contained (own capture modes + confirm). */}
      {scanMedia === 'music' && <MusicScan />}
      {scanMedia === 'game' && <GameCameraScan />}
      {scanMedia === 'card' && <CardScan />}

      {scanMedia === 'book' && (
        <>
      <div className="tabs" role="tablist" aria-label="Scan mode">
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'camera'}
          className={mode === 'camera' ? 'tab active' : 'tab'}
          onClick={() => setMode('camera')}
        >
          Camera
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'bulk'}
          className={mode === 'bulk' ? 'tab active' : 'tab'}
          onClick={() => setMode('bulk')}
        >
          Handheld scanner
        </button>
      </div>

      {mode === 'bulk' && <BookScan />}

      {mode === 'camera' && (
        <>
          {scanMedia === 'book' && (
            <>
              {!secure && (
                <div role="alert" className="callout">
                  <strong>Camera scanning needs a secure (HTTPS) connection.</strong>
                  <p style={{ margin: '0.5rem 0 0' }}>
                    Open OmniShelf over your Tailscale HTTPS address to scan barcodes. You can
                    still add books by entering the ISBN below.
                  </p>
                </div>
              )}

              {secure && cameraDenied && (
                <p role="alert" className="alert">
                  Camera access was blocked. Grant camera permission and reload, or enter the ISBN manually below.
                </p>
              )}

              {notFoundIsbn !== null && (
                <p role="alert" className="alert">
                  No book found for ISBN {notFoundIsbn}. Check the number below and try again, or enter it by hand.
                </p>
              )}

              {error !== null && (
                <p role="alert" className="alert">
                  {error}
                </p>
              )}

              {secure && !showManual && (
                <div className="stack">
                  <ScannerView onDetected={handleIsbn} onCameraError={() => setCameraDenied(true)} />
                  {looking && <p role="status" className="muted">Looking up…</p>}
                  <div>
                    <button type="button" className="btn-ghost" onClick={() => setManualMode(true)}>
                      Enter ISBN manually
                    </button>
                  </div>
                </div>
              )}

              {showManual && (
                <div className="stack">
                  <ManualIsbnForm
                    key={notFoundIsbn ?? 'manual'}
                    initialIsbn={notFoundIsbn ?? ''}
                    onSubmit={handleIsbn}
                    busy={looking}
                  />
                  {secure && (
                    <div>
                      <button type="button" className="btn-ghost" onClick={reset}>
                        Back to scanner
                      </button>
                    </div>
                  )}
                </div>
              )}
            </>
          )}
        </>
      )}
        </>
      )}
    </section>
  )
}