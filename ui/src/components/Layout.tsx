import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  isActive ? 'nav-link active' : 'nav-link'

export default function Layout() {
  const { user } = useAuth()

  return (
    <div className="app-shell">
      <nav className="topnav" aria-label="Main navigation">
        <span className="brand">OmniShelf</span>
        <NavLink to="/" className={navLinkClass} end>
          Up Next
        </NavLink>
        <NavLink to="/discover" className={navLinkClass}>
          Discover
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
        <span className="nav-spacer">
          {user !== null && (
            <NavLink to="/settings" className={navLinkClass} aria-label="Settings">
              {user.username}
            </NavLink>
          )}
        </span>
      </nav>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
