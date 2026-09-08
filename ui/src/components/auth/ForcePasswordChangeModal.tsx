import { useState } from 'react'
import type { FormEvent } from 'react'
import { changePassword, logout } from '../../api/auth'
import { useAuth } from '../../hooks/useAuth'

export default function ForcePasswordChangeModal() {
  const { refresh, clear } = useAuth()
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const validate = (): string | null => {
    if (newPassword.length < 8) return 'Password must be at least 8 characters long.'
    if (newPassword.length > 72) return 'Password must be at most 72 characters long.'
    if (newPassword !== confirmPassword) return 'Passwords do not match.'
    if (newPassword === 'admin') return 'Please choose a password other than the temporary one.'
    return null
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const validationError = validate()
    if (validationError) {
      setError(validationError)
      return
    }

    setSubmitting(true)
    setError(null)
    try {
      await changePassword(newPassword)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update password.')
    } finally {
      setSubmitting(false)
    }
  }

  const handleSignOut = async () => {
    try {
      await logout()
    } catch {
      // Ignore network errors on sign out
    }
    clear()
  }

  return (
    <div className="modal-backdrop" style={{ zIndex: 100 }}>
      <div className="modal" style={{ maxWidth: '420px', marginTop: '10vh' }}>
        <h2 style={{ marginTop: 0 }}>🔒 Password Change Required</h2>
        <p className="muted">
          Your password was recently reset. Please choose a new, secure password before accessing your shelf.
        </p>

        <form onSubmit={handleSubmit} noValidate>
          {error && (
            <p role="alert" className="alert" style={{ marginBottom: '1rem' }}>
              {error}
            </p>
          )}

          <label style={{ display: 'block', marginBottom: '0.75rem' }}>
            New Password
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="Minimum 8 characters"
              autoComplete="new-password"
              autoFocus
              required
              minLength={8}
              maxLength={72}
              style={{ width: '100%', marginTop: '0.25rem' }}
            />
          </label>

          <label style={{ display: 'block', marginBottom: '1.25rem' }}>
            Confirm New Password
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="Re-enter new password"
              autoComplete="new-password"
              required
              style={{ width: '100%', marginTop: '0.25rem' }}
            />
          </label>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
            <button type="submit" className="btn-primary" disabled={submitting}>
              {submitting ? 'Saving Password…' : 'Set New Password'}
            </button>
            <button type="button" className="btn-ghost" onClick={handleSignOut}>
              Sign out instead
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
