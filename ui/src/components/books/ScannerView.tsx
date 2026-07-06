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
      formatsToSupport: [Html5QrcodeSupportedFormats.EAN_13],
      verbose: false,
    })
    let detected = false

    scanner
      .start(
        { facingMode: 'environment' },
        { fps: 10, qrbox: { width: 250, height: 150 } },
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

  return <div id={SCANNER_ELEMENT_ID} data-testid="scanner-view" />
}
