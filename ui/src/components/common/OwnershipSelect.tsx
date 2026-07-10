/**
 * Ownership is a multi-select over a fixed set of physical formats. Music is
 * the first (and, for now, only) media type with ownership: an album can be
 * owned on Vinyl and/or CD. The component is generic over its option list so a
 * future media type can reuse it with a different vocabulary.
 */

/** The fixed music ownership vocabulary, matching the server's allowed set. */
export const MUSIC_OWNERSHIP = ['Vinyl', 'CD'] as const

interface OwnershipSelectProps {
  /** The allowed formats to offer (e.g. MUSIC_OWNERSHIP). */
  options: readonly string[]
  /** Currently-owned formats. */
  value: string[]
  /** Called with the next full set when a checkbox toggles. */
  onChange: (next: string[]) => void
  disabled?: boolean
  /** aria-label for the group. */
  label?: string
}

/**
 * A checkbox group for choosing which physical formats the user owns. Toggling
 * a box calls onChange with the complete next set (ordered by the options list)
 * so callers can persist it in one PATCH.
 */
export function OwnershipSelect({
  options,
  value,
  onChange,
  disabled = false,
  label = 'Ownership',
}: OwnershipSelectProps) {
  const owned = new Set(value)

  const toggle = (format: string) => {
    const next = new Set(owned)
    if (next.has(format)) next.delete(format)
    else next.add(format)
    // Preserve the options' order in the emitted array.
    onChange(options.filter((o) => next.has(o)))
  }

  return (
    <fieldset className="ownership-select" aria-label={label} style={{ border: 'none', padding: 0, margin: 0 }}>
      <legend className="muted" style={{ padding: 0 }}>
        {label}
      </legend>
      <div className="cluster" style={{ marginTop: '0.25rem' }}>
        {options.map((format) => (
          <label key={format} className="ownership-option cluster" style={{ gap: '0.35rem' }}>
            <input
              type="checkbox"
              checked={owned.has(format)}
              disabled={disabled}
              onChange={() => toggle(format)}
            />
            <span>{format}</span>
          </label>
        ))}
      </div>
    </fieldset>
  )
}

/** Read-only pills showing the owned formats; renders nothing when empty. */
export function OwnershipBadges({ value }: { value: string[] }) {
  if (value.length === 0) return null
  return (
    <span className="ownership-badges cluster" style={{ gap: '0.35rem' }}>
      {value.map((format) => (
        <span key={format} className="badge">
          {format}
        </span>
      ))}
    </span>
  )
}
