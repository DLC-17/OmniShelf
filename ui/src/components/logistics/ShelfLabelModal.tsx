import { useState } from 'react'
import { useLocations } from '../../hooks/useLogistics'
import { getLabelSheetURL } from '../../api/logistics'

interface ShelfLabelModalProps {
  onClose: () => void
  initialLocation?: string
}

export default function ShelfLabelModal({ onClose, initialLocation }: ShelfLabelModalProps) {
  const { data: locations, isLoading } = useLocations()
  const [selectedLocation, setSelectedLocation] = useState<string>(initialLocation || '')
  const [format, setFormat] = useState<'avery5160' | 'thermal4x6' | 'thermal2x1'>('thermal4x6')

  const labelUrl = getLabelSheetURL(format, selectedLocation || undefined)

  const handlePrint = () => {
    const printWindow = window.open(labelUrl, '_blank')
    if (printWindow) {
      printWindow.addEventListener('load', () => {
        printWindow.print()
      })
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose} role="dialog" aria-modal="true">
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: '780px', width: '90%' }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h2 style={{ margin: 0 }}>🏷️ Printable Storage & Shelf Labels</h2>
          <button type="button" className="btn-secondary" onClick={onClose} style={{ padding: '0.25rem 0.6rem' }}>
            ✕
          </button>
        </div>

        <p className="muted" style={{ fontSize: '0.88rem', marginTop: 0 }}>
          Generate printable QR and barcode labels for your physical bookshelves, storage boxes, media totes, and card binder sleeves. Scanning a label instantly opens that container's inventory.
        </p>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '1rem', marginBottom: '1.25rem' }}>
          <div>
            <label style={{ display: 'block', fontWeight: 600, fontSize: '0.85rem', marginBottom: '0.35rem' }}>
              Storage Location / Container:
            </label>
            <select
              value={selectedLocation}
              onChange={(e) => setSelectedLocation(e.target.value)}
              style={{ width: '100%', padding: '0.45rem' }}
              disabled={isLoading}
            >
              <option value="">All Storage Locations ({locations?.length || 0})</option>
              {locations?.map((loc) => (
                <option key={loc.location} value={loc.location}>
                  {loc.location} ({loc.itemCount} items - {loc.containerType})
                </option>
              ))}
            </select>
          </div>

          <div>
            <label style={{ display: 'block', fontWeight: 600, fontSize: '0.85rem', marginBottom: '0.35rem' }}>
              Label Format:
            </label>
            <select
              value={format}
              onChange={(e) => setFormat(e.target.value as any)}
              style={{ width: '100%', padding: '0.45rem' }}
            >
              <option value="thermal4x6">4" x 6" Storage Tote / Box Label (High-Density)</option>
              <option value="thermal2x1">2" x 1" Shelf Edge / Spine Label</option>
              <option value="avery5160">Avery 5160 (30 labels per Letter sheet)</option>
            </select>
          </div>
        </div>

        {/* SVG Preview Frame */}
        <div style={{
          background: '#ffffff',
          borderRadius: 'var(--radius-sm)',
          border: '1px solid var(--border)',
          padding: '1rem',
          minHeight: '260px',
          maxHeight: '380px',
          overflow: 'auto',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center'
        }}>
          <iframe
            src={labelUrl}
            title="Label Preview"
            style={{
              width: '100%',
              height: '340px',
              border: 'none',
              borderRadius: 'var(--radius-sm)',
            }}
          />
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.75rem', marginTop: '1.25rem' }}>
          <a
            href={labelUrl}
            download={`omnishelf-labels-${format}.svg`}
            className="btn-secondary"
            style={{ textDecoration: 'none', display: 'inline-flex', alignItems: 'center' }}
          >
            📥 Download SVG
          </a>
          <button type="button" className="btn-primary" onClick={handlePrint}>
            🖨️ Print Label Sheet
          </button>
        </div>
      </div>
    </div>
  )
}
