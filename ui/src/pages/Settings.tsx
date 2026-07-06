import { Link } from 'react-router-dom'
import { logout } from '../api/auth'
import { useAuth } from '../hooks/useAuth'

/**
 * User settings, reached by clicking the username in the nav. Holds account
 * info, the data-import entry point, and sign-out.
 */
export default function Settings() {
  const { user, clear } = useAuth()

  const handleLogout = async () => {
    try {
      await logout()
    } catch {
      // Even if the server call fails (e.g. session already expired), we still
      // drop local state; the cookie is HttpOnly so there is nothing else to do.
    }
    clear()
  }

  return (
    <section>
      <h1>Settings</h1>

      <div className="card">
        <h2>Account</h2>
        <p className="muted" style={{ margin: 0 }}>
          Signed in as <strong>{user?.username}</strong>
        </p>
      </div>

      <div className="card" style={{ marginTop: '1rem' }}>
        <h2>Import</h2>
        <p className="muted">Bring in your history from TV Time or Goodreads (CSV or zip export).</p>
        <Link to="/import" className="btn-primary" role="button">
          Import data
        </Link>
      </div>

      <div style={{ marginTop: '1.75rem' }}>
        <button type="button" onClick={handleLogout}>
          Sign out
        </button>
      </div>
    </section>
  )
}
