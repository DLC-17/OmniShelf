/**
 * Camera/photo → upload-ready JPEG for the card scanner. Kept separate from
 * the CardScan component so the geometry stays unit-testable and component
 * tests can mock the canvas-backed encoders (jsdom implements neither canvas
 * 2D nor image decoding).
 */

/** Longest side of the uploaded JPEG; the identify pipeline needs no more. */
export const MAX_DIMENSION = 1024

/** JPEG encode quality for the upload. */
export const JPEG_QUALITY = 0.85

/**
 * The target size for a width×height source: scaled down preserving aspect
 * ratio so the longest side is ≤ max. Sources already within the limit come
 * back unchanged — never upscaled.
 */
export function scaleToFit(
  width: number,
  height: number,
  max: number = MAX_DIMENSION,
): { width: number; height: number } {
  const longest = Math.max(width, height)
  if (longest <= max) return { width, height }
  const factor = max / longest
  return {
    width: Math.max(1, Math.round(width * factor)),
    height: Math.max(1, Math.round(height * factor)),
  }
}

/** Draws source at its scaled-to-fit size and encodes an upload-ready JPEG. */
export function compressToJpeg(
  source: CanvasImageSource,
  sourceWidth: number,
  sourceHeight: number,
): Promise<Blob> {
  const { width, height } = scaleToFit(sourceWidth, sourceHeight)
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const context = canvas.getContext('2d')
  if (context === null) {
    return Promise.reject(new Error('This browser does not support canvas drawing'))
  }
  context.drawImage(source, 0, 0, width, height)
  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (blob !== null) resolve(blob)
        else reject(new Error('Could not encode the photo'))
      },
      'image/jpeg',
      JPEG_QUALITY,
    )
  })
}

/** Grabs the current frame of a live camera preview as an upload-ready JPEG. */
export function captureVideoFrame(video: HTMLVideoElement): Promise<Blob> {
  if (video.videoWidth === 0 || video.videoHeight === 0) {
    return Promise.reject(new Error('The camera has not produced a frame yet'))
  }
  return compressToJpeg(video, video.videoWidth, video.videoHeight)
}

/** Decodes an uploaded image file and re-encodes it as an upload-ready JPEG. */
export async function fileToJpegBlob(file: File): Promise<Blob> {
  if (typeof createImageBitmap === 'function') {
    const bitmap = await createImageBitmap(file)
    try {
      return await compressToJpeg(bitmap, bitmap.width, bitmap.height)
    } finally {
      bitmap.close()
    }
  }

  // Older browsers: decode through an <img> and an object URL.
  const url = URL.createObjectURL(file)
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image()
      el.onload = () => resolve(el)
      el.onerror = () => reject(new Error('Could not read that image'))
      el.src = url
    })
    return await compressToJpeg(image, image.naturalWidth, image.naturalHeight)
  } finally {
    URL.revokeObjectURL(url)
  }
}
