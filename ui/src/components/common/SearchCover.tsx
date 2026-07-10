import { useState } from 'react'

interface SearchCoverProps {
  /** Same-origin cover-proxy path (e.g. `/api/covers/game/co3p2d`), or null when
   * the result has no cover source. */
  src: string | null
  title: string
}

/**
 * Cover thumbnail for a search result. Search hits aren't tracked yet, so their
 * art isn't cached under /images; instead we load it through the same-origin
 * cover proxy (CSP-safe). Falls back to an initial-letter placeholder when there
 * is no cover source or the image fails to load.
 */
export default function SearchCover({ src, title }: SearchCoverProps) {
  const [failed, setFailed] = useState(false)

  if (src === null || src === '' || failed) {
    return (
      <div
        role="img"
        aria-label={`No cover for ${title}`}
        className="poster placeholder"
        style={{ width: 46, height: 69, fontSize: '1rem' }}
      >
        {title.charAt(0).toUpperCase()}
      </div>
    )
  }

  return (
    <img
      src={src}
      alt={`Cover of ${title}`}
      className="poster"
      width={46}
      height={69}
      loading="lazy"
      style={{ objectFit: 'cover' }}
      onError={() => setFailed(true)}
    />
  )
}
