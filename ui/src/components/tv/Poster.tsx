import { useState } from 'react'

interface PosterProps {
  /** Relative path under the images dir; empty string means "no cached poster". */
  posterPath: string
  title: string
  width?: number
  height?: number
}

/**
 * Show poster with placeholder fallback: posters are served from
 * the local /images cache; a missing path or a failed load renders a neutral
 * placeholder instead of a broken image.
 */
export default function Poster({ posterPath, title, width = 80, height = 120 }: PosterProps) {
  const [failed, setFailed] = useState(false)

  if (posterPath === '' || failed) {
    return (
      <div
        role="img"
        aria-label={`No poster for ${title}`}
        className="poster placeholder"
        style={{ width, height }}
      >
        {title.charAt(0).toUpperCase()}
      </div>
    )
  }

  return (
    <img
      src={`/images/${posterPath}`}
      alt={`Poster for ${title}`}
      width={width}
      height={height}
      className="poster"
      onError={() => setFailed(true)}
    />
  )
}
