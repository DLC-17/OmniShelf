import { useState } from 'react'

interface PosterProps {
  /** Relative path under the images dir; empty string means "no cached poster". */
  posterPath: string
  title: string
  width?: number | string
  height?: number | string
  className?: string
  style?: React.CSSProperties
}

const TMDB_THUMB = 'https://image.tmdb.org/t/p/w185'

/**
 * Show poster with placeholder fallback: posters are served from
 * the local /images cache or TMDB CDN; a missing path or a failed load renders a neutral
 * placeholder instead of a broken image.
 */
export default function Poster({ posterPath, title, width, height, className, style }: PosterProps) {
  const [failed, setFailed] = useState(false)

  const combinedStyle: React.CSSProperties = {
    ...(width !== undefined ? { width } : {}),
    ...(height !== undefined ? { height } : {}),
    ...style,
  }

  if (posterPath === '' || failed) {
    return (
      <div
        role="img"
        aria-label={`No poster for ${title}`}
        className={`poster placeholder ${className || ''}`}
        style={combinedStyle}
      >
        {title ? title.charAt(0).toUpperCase() : '?'}
      </div>
    )
  }

  const imageSrc =
    posterPath.startsWith('http://') || posterPath.startsWith('https://')
      ? posterPath
      : posterPath.startsWith('//')
        ? `https:${posterPath}`
        : posterPath.startsWith('/images/')
          ? posterPath
          : posterPath.startsWith('/')
            ? `${TMDB_THUMB}${posterPath}`
            : `/images/${posterPath}`

  return (
    <img
      src={imageSrc}
      alt={`Poster for ${title}`}
      className={`poster ${className || ''}`}
      style={combinedStyle}
      loading="lazy"
      onError={() => setFailed(true)}
    />
  )
}
