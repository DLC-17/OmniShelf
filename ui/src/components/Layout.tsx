import { NavLink, Outlet } from 'react-router-dom'
import { logout } from '../api/auth'
import { useAuth } from '../hooks/useAuth'

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  isActive ? 'nav-link active' : 'nav-link'

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
    <div className="app-shell">
      <nav className="topnav" aria-label="Main navigation">
        <span className="brand">OmniShelf</span>
        <NavLink to="/" className={navLinkClass} end>
          Up Next
        </NavLink>
        <NavLink to="/library" className={navLinkClass}>
          Library
        </NavLink>
        <NavLink to="/scan" className={navLinkClass}>
          Scan
        </NavLink>
        <NavLink to="/feed" className={navLinkClass}>
          Feed
        </NavLink>
        <NavLink to="/import" className={navLinkClass}>
          Import
        </NavLink>
        <span className="nav-spacer">
          {user !== null && <span className="nav-user">{user.username}</span>}
          <button type="button" className="btn-ghost" onClick={handleLogout}>
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
