import { useEffect, useRef, useState } from 'react'
import type { ChangeEvent } from 'react'
import { ApiError } from '../../api/client'
import { scanCard } from '../../api/cards'
import type { Card } from '../../api/cards'
import { captureVideoFrame, fileToJpegBlob } from '../../lib/cardImage'
import { isSecureContext } from '../../lib/secureContext'
import CardConfirmCard from './CardConfirmCard'

/** Human-readable explanation for each identify miss the backend reports. */
function scanErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'no_text_detected') {
      return 'No text detected — try better lighting or a closer shot.'
    }
    if (err.code === 'unsupported_card') {
      return "Couldn't recognize this card type or set code. Try a sharper, straight-on photo."
    }
    if (err.code === 'card_not_found') {
      const setCode = typeof err.details.setCode === 'string' ? err.details.setCode : ''
      return setCode !== ''
        ? `No card found for set code ${setCode}. Try another photo.`
        : 'No matching card found. Try another photo.'
    }
    return err.message
  }
  return 'Something went wrong. Please try again.'
}

/**
 * The trading-card scanning experience: photograph a card with the live
 * environment-facing camera (secure contexts only), or upload a photo of it
 * on plain HTTP / camera denial. Either source is downscaled client-side to a
 * ≤1024px JPEG and POSTed to /api/cards/scan; a successful identify hands off
 * to CardConfirmCard, and misses render a retryable explanation.
 */
export default function CardScan() {
  const secure = isSecureContext()

  const [card, setCard] = useState<Card | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [cameraDenied, setCameraDenied] = useState(false)
  const [uploadMode, setUploadMode] = useState(false)
  const videoRef = useRef<HTMLVideoElement>(null)

  const showUpload = !secure || cameraDenied || uploadMode
  const cameraActive = secure && !showUpload && card === null

  // Live camera preview. Only started on a secure context with permission;
  // the cleanup stops the tracks whenever the preview leaves the screen
  // (confirm step, upload fallback, unmount).
  useEffect(() => {
    if (!cameraActive) return
    let cancelled = false
    let stream: MediaStream | null = null

    const start = async () => {
      if (navigator.mediaDevices === undefined) {
        setCameraDenied(true)
        return
      }
      try {
        stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } })
      } catch {
        // getUserMedia rejects when the user denies the camera or none exists.
        if (!cancelled) setCameraDenied(true)
        return
      }
      if (cancelled) {
        stream.getTracks().forEach((track) => track.stop())
        return
      }
      const video = videoRef.current
      if (video !== null) {
        video.srcObject = stream
        try {
          await video.play()
        } catch {
          // Autoplay rejection; the muted playsInline preview starts on interaction.
        }
      }
    }

    void start()
    return () => {
      cancelled = true
      stream?.getTracks().forEach((track) => track.stop())
    }
  }, [cameraActive])

  const identify = async (image: Blob) => {
    setError(null)
    setBusy(true)
    try {
      setCard(await scanCard(image))
    } catch (err) {
      setError(scanErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const handleCapture = async () => {
    const video = videoRef.current
    if (video === null || busy) return
    let image: Blob
    try {
      image = await captureVideoFrame(video)
    } catch {
      setError('Could not capture a photo — give the camera a moment, or upload a photo instead.')
      return
    }
    await identify(image)
  }

  const handleFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = '' // allow re-selecting the same file after a miss
    if (file === undefined) return
    let image: Blob
    try {
      image = await fileToJpegBlob(file)
    } catch {
      setError('Could not read that image. Try a different photo.')
      return
    }
    await identify(image)
  }

  const reset = () => {
    setCard(null)
    setError(null)
  }

  if (card !== null) {
    return <CardConfirmCard card={card} onDone={reset} />
  }

  return (
    <div className="stack">
      {!secure && (
        <div role="alert" className="callout">
          <strong>Camera capture needs a secure (HTTPS) connection.</strong>
          <p style={{ margin: '0.5rem 0 0' }}>
            Open OmniShelf over your Tailscale HTTPS address to photograph cards with the camera.
            You can still identify a card by uploading a photo of it below.
          </p>
        </div>
      )}

      {secure && cameraDenied && (
        <p role="alert" className="alert">
          Camera access was blocked. Grant camera permission and reload, or upload a photo of the
          card below.
        </p>
      )}

      {error !== null && (
        <p role="alert" className="alert">
          {error}
        </p>
      )}

      {cameraActive && (
        <div className="stack">
          <video
            ref={videoRef}
            autoPlay
            playsInline
            muted
            data-testid="card-camera"
            style={{ width: '100%', maxWidth: '28rem', borderRadius: '0.5rem' }}
          />
          <div className="cluster">
            <button type="button" className="btn-primary" onClick={handleCapture} disabled={busy}>
              Capture card
            </button>
            <button type="button" className="btn-ghost" onClick={() => setUploadMode(true)} disabled={busy}>
              Upload a photo instead
            </button>
          </div>
        </div>
      )}

      {showUpload && (
        <div className="stack">
          <label className="field">
            <span>Card photo</span>
            <input
              type="file"
              accept="image/*"
              capture="environment"
              aria-label="Card photo"
              onChange={handleFile}
              disabled={busy}
            />
          </label>
          {secure && !cameraDenied && (
            <div>
              <button type="button" className="btn-ghost" onClick={() => setUploadMode(false)} disabled={busy}>
                Back to camera
              </button>
            </div>
          )}
        </div>
      )}

      {busy && (
        <p role="status" className="muted">
          Identifying card…
        </p>
      )}
    </div>
  )
}
