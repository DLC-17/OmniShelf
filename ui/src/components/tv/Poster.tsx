import { useState } from 'react'

interface PosterProps {
  /** Relative path under the images dir; empty string means "no cached poster". */
  posterPath: string
  title: string
  width?: number
  height?: number
}

/**
 * Show poster with placeholder fallback (spec §2.8): posters are served from
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
        style={{
          width,
          height,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: '#ddd',
          color: '#666',
          borderRadius: 4,
          fontSize: '1.5rem',
          flexShrink: 0,
        }}
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
      style={{ objectFit: 'cover', borderRadius: 4, flexShrink: 0 }}
      onError={() => setFailed(true)}
    />
  )
}
