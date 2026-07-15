/**
 * Checks whether the user is browsing from a mobile device (phone or tablet).
 * Used to avoid triggering automatic camera/webcam permission prompts on desktop.
 */
export function isMobileDevice(): boolean {
  const userAgent = navigator.userAgent || navigator.vendor;
  if (/android|ipad|iphone|ipod|windows phone/i.test(userAgent)) {
    return true;
  }
  // iPad Pro running iPadOS (which reports as MacIntel but has touch points)
  if (navigator.maxTouchPoints && navigator.maxTouchPoints > 1 && /MacIntel/.test(navigator.platform)) {
    return true;
  }
  return false;
}
