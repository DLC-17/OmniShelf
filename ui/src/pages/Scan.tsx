import { useState } from 'react'
import { ApiError } from '../api/client'
import { scanBook } from '../api/books'
import type { Book } from '../api/books'
import { isSecureContext } from '../lib/secureContext'
import { bookScanTarget } from '../lib/scanTargets'
import ScannerView from '../components/books/ScannerView'
import BookConfirmCard from '../components/books/BookConfirmCard'
import BulkScanner from '../components/books/BulkScanner'
import ManualIsbnForm from '../components/books/ManualIsbnForm'
import GameScan from '../components/games/GameScan'

type ScanMode = 'camera' | 'bulk'
type ScanMedia = 'book' | 'game'

export default function Scan() {
  const secure = isSecureContext()

  const [scanMedia, setScanMedia] = useState<ScanMedia>('book')
  const [mode, setMode] = useState<ScanMode>('camera')
  const [book, setBook] = useState<Book | null>(null)
  const [looking, setLooking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // The ISBN that returned 404, used to pre-fill and explain manual entry (E4).
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
        // E4: unknown ISBN — drop into manual entry pre-filled with this ISBN.
        setNotFoundIsbn(isbn)
      } else if (err instanceof ApiError) {
        // E-upstream: OpenLibrary 502, or any other API error.
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

  // Manual entry is forced by an insecure context (E6), camera denial (E7) or a
  // 404 (E4); otherwise it is an opt-in alternative to the camera.
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
      </div>

      {scanMedia === 'game' && <GameScan />}

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

      {mode === 'bulk' && <BulkScanner target={bookScanTarget} />}

      {mode === 'camera' && (
        <>
      {!secure && (
        <div role="alert" className="callout">
          <strong>Camera scanning needs a secure (HTTPS) connection.</strong>
          <p style={{ margin: '0.5rem 0 0' }}>
            Open OmniShelf over your Tailscale HTTPS address (https://…) to scan barcodes. You can
            still add books by entering the ISBN below.
          </p>
        </div>
      )}

      {secure && cameraDenied && (
        <p role="alert" className="alert">
          Camera access was blocked. Grant camera permission and reload, or enter the ISBN manually
          below.
        </p>
      )}

      {notFoundIsbn !== null && (
        <p role="alert" className="alert">
          No book found for ISBN {notFoundIsbn}. Check the number below and try again, or enter it
          by hand.
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
    </section>
  )
}
