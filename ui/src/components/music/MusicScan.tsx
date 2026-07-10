import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import { scanAlbum } from '../../api/music'
import type { Album } from '../../api/music'
import { isSecureContext } from '../../lib/secureContext'
import { musicScanTarget } from '../../lib/scanTargets'
import BulkScanner from '../books/BulkScanner'
import ScannerView from '../books/ScannerView'
import AlbumConfirmCard from './AlbumConfirmCard'

type ScanMode = 'camera' | 'bulk'

/**
 * The music scanning experience: a camera EAN/UPC scanner that resolves an
 * album through Discogs into a confirm card, plus a handheld bulk-scanner mode
 * for running a stack of cases through quickly. Mirrors the game scanner. On an
 * insecure context or camera denial it falls back to manual barcode entry.
 */
export default function MusicScan() {
  const secure = isSecureContext()

  const [mode, setMode] = useState<ScanMode>('bulk')
  const [album, setAlbum] = useState<Album | null>(null)
  const [looking, setLooking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notFound, setNotFound] = useState<string | null>(null)
  const [cameraDenied, setCameraDenied] = useState(false)
  const [manualValue, setManualValue] = useState('')

  const handleBarcode = async (barcode: string) => {
    setError(null)
    setNotFound(null)
    setLooking(true)
    try {
      setAlbum(await scanAlbum(barcode))
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setNotFound(barcode)
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
    setAlbum(null)
    setError(null)
    setNotFound(null)
    setCameraDenied(false)
    setManualValue('')
  }

  if (album !== null) {
    return <AlbumConfirmCard album={album} onDone={reset} />
  }

  const submitManual = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const trimmed = manualValue.trim()
    if (trimmed !== '') void handleBarcode(trimmed)
  }

  const showManual = !secure || cameraDenied

  return (
    <div className="stack">
      <div className="tabs" role="tablist" aria-label="Album scan mode">
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'bulk'}
          className={mode === 'bulk' ? 'tab active' : 'tab'}
          onClick={() => setMode('bulk')}
        >
          Handheld scanner
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={mode === 'camera'}
          className={mode === 'camera' ? 'tab active' : 'tab'}
          onClick={() => setMode('camera')}
        >
          Camera
        </button>
      </div>

      {mode === 'bulk' && <BulkScanner target={musicScanTarget} />}

      {mode === 'camera' && (
        <>
          {!secure && (
            <div role="alert" className="callout">
              <strong>Camera scanning needs a secure (HTTPS) connection.</strong>
              <p style={{ margin: '0.5rem 0 0' }}>
                Open OmniShelf over your Tailscale HTTPS address to scan with the camera. You can
                still add albums by entering the barcode below or using the handheld scanner.
              </p>
            </div>
          )}

          {secure && cameraDenied && (
            <p role="alert" className="alert">
              Camera access was blocked. Grant camera permission and reload, or enter the barcode
              manually below.
            </p>
          )}

          {notFound !== null && (
            <p role="alert" className="alert">
              No album found for barcode {notFound}. Check the number and try again.
            </p>
          )}

          {error !== null && (
            <p role="alert" className="alert">
              {error}
            </p>
          )}

          {secure && !cameraDenied && (
            <ScannerView onDetected={handleBarcode} onCameraError={() => setCameraDenied(true)} />
          )}

          {looking && <p role="status" className="muted">Looking up…</p>}

          {showManual && (
            <form className="searchbar" onSubmit={submitManual}>
              <label className="field grow">
                <span>Barcode</span>
                <input
                  type="text"
                  inputMode="numeric"
                  aria-label="Barcode"
                  placeholder="602547790392"
                  value={manualValue}
                  onChange={(e) => setManualValue(e.target.value)}
                  disabled={looking}
                  style={{ flex: 1 }}
                />
              </label>
              <button type="submit" className="btn-primary" disabled={looking || manualValue.trim() === ''}>
                {looking ? 'Looking up…' : 'Look up'}
              </button>
            </form>
          )}
        </>
      )}
    </div>
  )
}
