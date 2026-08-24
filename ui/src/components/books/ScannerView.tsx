import { useEffect, useRef } from 'react'
import { Html5Qrcode, Html5QrcodeSupportedFormats } from 'html5-qrcode'

/** DOM id the html5-qrcode library mounts the camera preview into. */
const SCANNER_ELEMENT_ID = 'omnishelf-scanner'

interface ScannerViewProps {
  /** Called once with the decoded EAN-13/ISBN-13 string on the first hit. */
  onDetected: (isbn: string) => void
  /** Called when the camera cannot start (permission denied or unavailable, E7). */
  onCameraError: () => void
}

/**
 * html5-qrcode EAN-13 scanner. Rendered only inside a Secure Context (E6); the
 * parent gates on `isSecureContext()` so this component — and the underlying
 * getUserMedia call — never runs on plain HTTP.
 */
export default function ScannerView({ onDetected, onCameraError }: ScannerViewProps) {
  // Callbacks are read through refs so the start/stop effect can run exactly
  // once on mount without restarting the camera when a parent re-renders.
  const onDetectedRef = useRef(onDetected)
  const onCameraErrorRef = useRef(onCameraError)
  useEffect(() => {
    onDetectedRef.current = onDetected
    onCameraErrorRef.current = onCameraError
  })

  useEffect(() => {
    const scanner = new Html5Qrcode(SCANNER_ELEMENT_ID, {
      formatsToSupport: [
        Html5QrcodeSupportedFormats.EAN_13,
        Html5QrcodeSupportedFormats.EAN_8,
        Html5QrcodeSupportedFormats.UPC_A,
      ],
      verbose: false,
      // Use the browser's native BarcodeDetector when available (Chromium);
      // falls back to ZXing JS on Firefox / Safari automatically.
      experimentalFeatures: {
        useBarCodeDetectorIfSupported: true,
      },
    })
    let detected = false

    scanner
      .start(
        { facingMode: 'environment' },
        {
          fps: 10,
          // Scale the decode region to the viewfinder so it works on both a
          // narrow phone screen and a wide 16:9 webcam feed.  Floors ensure
          // the box never collapses below a usable size on tiny viewports.
          qrbox: (viewfinderWidth: number, viewfinderHeight: number) => ({
            width: Math.max(250, Math.floor(viewfinderWidth * 0.7)),
            height: Math.max(150, Math.floor(viewfinderHeight * 0.35)),
          }),
          // Skip the mirror-flip check — barcodes are always read from the
          // rear camera (mobile) or a forward-facing webcam (desktop).
          disableFlip: true,
          // Ask for 720p and continuous autofocus; `ideal` and `advanced` are
          // hints — phones/webcams that don't support them ignore gracefully.
          // When videoConstraints is present the library uses it as-is for
          // getUserMedia, so facingMode is repeated here.
          videoConstraints: {
            facingMode: 'environment',
            width: { ideal: 1280 },
            height: { ideal: 720 },
            advanced: [{ focusMode: 'continuous' } as MediaTrackConstraintSet],
          },
        },
        (decodedText) => {
          if (detected) return
          detected = true
          onDetectedRef.current(decodedText)
        },
        // Per-frame decode misses fire constantly and are not errors; ignore them.
        undefined,
      )
      .catch(() => {
        // start() rejects when the user denies the camera or none is available (E7).
        onCameraErrorRef.current()
      })

    return () => {
      if (scanner.isScanning) {
        scanner
          .stop()
          .catch(() => undefined)
          .finally(() => scanner.clear())
      }
    }
  }, [])

  return (
    <div className="scanner-view-container">
      <div id={SCANNER_ELEMENT_ID} data-testid="scanner-view" />
      <div className="scanner-overlay" aria-hidden="true">
        <div className="scanner-mask-top"></div>
        <div className="scanner-mask-left"></div>
        <div className="scanner-mask-right"></div>
        <div className="scanner-mask-bottom"></div>
        <div className="scanner-target-box">
          <div className="scanner-corner scanner-corner-tl"></div>
          <div className="scanner-corner scanner-corner-tr"></div>
          <div className="scanner-corner scanner-corner-bl"></div>
          <div className="scanner-corner scanner-corner-br"></div>
          <div className="scanner-laser"></div>
        </div>
        <div className="scanner-instruction">Align barcode within the guides</div>
      </div>
    </div>
  )
}
