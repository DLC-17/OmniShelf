/**
 * Camera access (barcode scanning) requires a Secure Context (E6). On plain
 * HTTP (LAN IP) this is false; the Scan page shows a banner pointing at the
 * Tailscale HTTPS origin instead of a broken camera view.
 */
export function isSecureContext(): boolean {
  return window.isSecureContext
}
