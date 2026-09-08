import { useCallback, useEffect, useState } from 'react'
import { fetchAdminUsers, resetUserPassword, toggleUserAdmin } from '../../api/admin'
import type { AdminUser } from '../../api/admin'
import { useAuth } from '../../hooks/useAuth'

export default function AdminUserManagement() {
  const { user: currentUser } = useAuth()
  const [users, setUsers] = useState<AdminUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [actionMessage, setActionMessage] = useState<string | null>(null)
  const [confirmResetId, setConfirmResetId] = useState<number | null>(null)

  const loadUsers = useCallback(async () => {
    try {
      const data = await fetchAdminUsers()
      setUsers(data)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load users.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadUsers()
  }, [loadUsers])

  const handleResetPassword = async (targetId: number, targetUsername: string) => {
    try {
      await resetUserPassword(targetId)
      setActionMessage(`Password for "${targetUsername}" reset to "admin".`)
      setConfirmResetId(null)
      await loadUsers()
    } catch (err) {
      setActionMessage(err instanceof Error ? err.message : 'Failed to reset password.')
    }
  }

  const handleToggleAdmin = async (targetId: number) => {
    try {
      await toggleUserAdmin(targetId)
      setActionMessage(null)
      await loadUsers()
    } catch (err) {
      setActionMessage(err instanceof Error ? err.message : 'Failed to update admin status.')
    }
  }

  if (loading) {
    return (
      <div className="card" style={{ marginTop: '1.5rem' }}>
        <p className="muted">Loading user administration…</p>
      </div>
    )
  }

  if (error) {
    return (
      <div className="card" style={{ marginTop: '1.5rem' }}>
        <p className="alert">{error}</p>
      </div>
    )
  }

  return (
    <div className="card" style={{ marginTop: '1.5rem' }}>
      <h2>🛡️ Admin: User Management</h2>
      <p className="muted">
        Manage instance users, reset passwords for locked accounts, and adjust administrator privileges.
      </p>

      {actionMessage && (
        <p
          className="muted"
          style={{
            marginTop: '0.75rem',
            padding: '0.65rem 0.85rem',
            background: 'var(--surface-alt)',
            borderRadius: 'var(--radius-sm)',
          }}
        >
          {actionMessage}
        </p>
      )}

      <table style={{ width: '100%', marginTop: '1rem', borderCollapse: 'collapse' }}>
        <thead>
          <tr style={{ borderBottom: '1px solid var(--border)', textAlign: 'left' }}>
            <th style={{ padding: '0.5rem' }}>Username</th>
            <th style={{ padding: '0.5rem' }}>Role</th>
            <th style={{ padding: '0.5rem' }}>Status</th>
            <th style={{ padding: '0.5rem', textAlign: 'right' }}>Actions</th>
          </tr>
        </thead>
        <tbody>
          {users.map((u) => {
            const isSelf = u.id === currentUser?.id
            return (
              <tr key={u.id} style={{ borderBottom: '1px solid var(--border)' }}>
                <td style={{ padding: '0.5rem' }}>
                  {u.username}
                  {isSelf && <span className="muted" style={{ fontSize: '0.8rem' }}> (you)</span>}
                </td>
                <td style={{ padding: '0.5rem' }}>
                  <span
                    className="badge"
                    style={u.isAdmin ? { borderColor: 'var(--accent)', color: 'var(--text)' } : {}}
                  >
                    {u.isAdmin ? '🛡️ Admin' : 'User'}
                  </span>
                </td>
                <td style={{ padding: '0.5rem' }}>
                  {u.mustChangePassword && (
                    <span
                      className="badge"
                      style={{ borderColor: 'var(--danger)', color: 'var(--danger)' }}
                    >
                      ⚠️ Password Reset Pending
                    </span>
                  )}
                </td>
                <td style={{ padding: '0.5rem', textAlign: 'right' }}>
                  <div style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end', flexWrap: 'wrap' }}>
                    {!isSelf && (
                      <>
                        {confirmResetId === u.id ? (
                          <div style={{ display: 'flex', gap: '0.35rem', alignItems: 'center' }}>
                            <span className="muted" style={{ fontSize: '0.82rem' }}>
                              Reset to 'admin'?
                            </span>
                            <button
                              type="button"
                              className="btn-danger"
                              style={{ padding: '0.25rem 0.6rem', fontSize: '0.82rem' }}
                              onClick={() => handleResetPassword(u.id, u.username)}
                            >
                              Confirm
                            </button>
                            <button
                              type="button"
                              className="btn-secondary"
                              style={{ padding: '0.25rem 0.6rem', fontSize: '0.82rem' }}
                              onClick={() => setConfirmResetId(null)}
                            >
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <button
                            type="button"
                            className="btn-secondary"
                            style={{ padding: '0.25rem 0.6rem', fontSize: '0.82rem' }}
                            onClick={() => setConfirmResetId(u.id)}
                          >
                            Reset Password
                          </button>
                        )}
                        <button
                          type="button"
                          className={u.isAdmin ? 'btn-secondary' : 'btn-confirm'}
                          style={{ padding: '0.25rem 0.6rem', fontSize: '0.82rem' }}
                          onClick={() => handleToggleAdmin(u.id)}
                        >
                          {u.isAdmin ? 'Demote' : 'Promote to Admin'}
                        </button>
                      </>
                    )}
                  </div>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
