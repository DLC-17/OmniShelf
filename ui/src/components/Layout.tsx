import { NavLink, Outlet } from 'react-router-dom'
import { logout } from '../api/auth'
import { useAuth } from '../hooks/useAuth'

const navLinkStyle = ({ isActive }: { isActive: boolean }) => ({
  marginRight: '1rem',
  textDecoration: 'none',
  fontWeight: isActive ? 700 : 400,
})

export default function Layout() {
  const { user, clear } = useAuth()

  const handleLogout = async () => {
    try {
      await logout()
    } catch {
      // Even if the server call fails (e.g. session already expired), we still
      // drop local state; the cookie is HttpOnly so there is nothing else to do.
    }
    // Clearing auth state makes the route guard redirect to /login.
    clear()
  }

  return (
    <div style={{ fontFamily: 'system-ui, sans-serif', maxWidth: 960, margin: '0 auto', padding: '0 1rem' }}>
      <nav
        aria-label="Main navigation"
        style={{ display: 'flex', alignItems: 'center', padding: '1rem 0', flexWrap: 'wrap' }}
      >
        <NavLink to="/" style={navLinkStyle} end>
          Up Next
        </NavLink>
        <NavLink to="/library" style={navLinkStyle}>
          Library
        </NavLink>
        <NavLink to="/scan" style={navLinkStyle}>
          Scan
        </NavLink>
        <NavLink to="/feed" style={navLinkStyle}>
          Feed
        </NavLink>
        <NavLink to="/import" style={navLinkStyle}>
          Import
        </NavLink>
        <span style={{ marginLeft: 'auto' }}>
          {user !== null && <span style={{ marginRight: '0.75rem' }}>{user.username}</span>}
          <button type="button" onClick={handleLogout}>
            Sign out
          </button>
        </span>
      </nav>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
