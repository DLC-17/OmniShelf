import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  isActive ? 'nav-link active' : 'nav-link'

/** One nav item: an icon (shown as a bottom-tab glyph on phones) + a label. */
function NavItem({ to, icon, label, end }: { to: string; icon: string; label: string; end?: boolean }) {
  return (
    <NavLink to={to} className={navLinkClass} end={end}>
      <span className="nav-icon" aria-hidden="true">
        {icon}
      </span>
      <span className="nav-label">{label}</span>
    </NavLink>
  )
}

export default function Layout() {
  const { user } = useAuth()

  return (
    <div className="app-shell">
      <nav className="topnav" aria-label="Main navigation">
        <span className="brand">OmniShelf</span>
        <NavItem to="/" icon="📺" label="Up Next" end />
        <NavItem to="/discover" icon="🧭" label="Discover" />
        <NavItem to="/library" icon="📚" label="Library" />
        <NavItem to="/scan" icon="📷" label="Scan" />
        <NavItem to="/feed" icon="👥" label="Feed" />
        <span className="nav-spacer">
          {user !== null && (
            <NavLink to="/settings" className={navLinkClass} aria-label="Settings">
              <span className="nav-icon" aria-hidden="true">
                ⚙️
              </span>
              <span className="nav-label">{user.username}</span>
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
