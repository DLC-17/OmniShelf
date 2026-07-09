/**
 * Reusable multi-select over a FIXED option set, rendered as toggleable badge
 * chips. It is media-agnostic: the caller supplies the allowed options and the
 * current selection, so games use it with {Physical, GOG} and #11 will reuse it
 * verbatim with {Vinyl, CD}. `onChange` receives the full next selection.
 */

interface OwnershipSelectProps {
  /** Fixed, ordered option set to toggle among. */
  options: string[]
  /** Currently selected options (any subset of `options`). */
  selected: string[]
  /** Called with the full next selection whenever a chip is toggled. */
  onChange: (next: string[]) => void
  disabled?: boolean
  /** Accessible group label; defaults to "Ownership". */
  label?: string
}

export default function OwnershipSelect({
  options,
  selected,
  onChange,
  disabled = false,
  label = 'Ownership',
}: OwnershipSelectProps) {
  const toggle = (opt: string) => {
    onChange(selected.includes(opt) ? selected.filter((o) => o !== opt) : [...selected, opt])
  }

  return (
    <div className="tag-list" role="group" aria-label={label}>
      {options.map((opt) => {
        const on = selected.includes(opt)
        return (
          <button
            key={opt}
            type="button"
            role="checkbox"
            aria-checked={on}
            className={on ? 'badge badge-ok badge-toggle' : 'badge badge-toggle'}
            disabled={disabled}
            onClick={() => toggle(opt)}
          >
            {opt}
          </button>
        )
      })}
    </div>
  )
}

/**
 * Read-only badge renderer for a selected ownership set. Renders nothing when
 * empty. Pairs with OwnershipSelect for display-only surfaces.
 */
export function OwnershipBadges({ formats }: { formats: string[] }) {
  if (formats.length === 0) return null
  return (
    <div className="tag-list">
      {formats.map((f) => (
        <span key={f} className="badge badge-ok">
          {f}
        </span>
      ))}
    </div>
  )
}
