import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import CommandPalette from './CommandPalette'

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
  const [isPaletteOpen, setIsPaletteOpen] = useState(false)

  return (
    <div className="app-shell">
      <nav className="topnav" aria-label="Main navigation">
        <span className="brand">OmniShelf</span>
        <NavItem to="/" icon="📺" label="Up Next" end />
        <NavItem to="/discover" icon="🧭" label="Discover" />
        <NavItem to="/library" icon="📚" label="Library" />
        <NavItem to="/scan" icon="📷" label="Scan" />
        <button
          type="button"
          className="nav-search-btn"
          onClick={() => setIsPaletteOpen(true)}
          aria-label="Search and command palette (Ctrl+K / Cmd+K)"
          title="Search and commands (Ctrl+K)"
        >
          <span aria-hidden="true">🔍</span>
          <span className="nav-search-label">Search</span>
          <kbd className="nav-kbd">⌘K</kbd>
        </button>
        <span className="nav-spacer">
          {user !== null && (
            <NavLink to="/settings" className={navLinkClass} aria-label="Profile & Settings">
              <span className="nav-icon" aria-hidden="true">
                👤
              </span>
              <span className="nav-label">{user.username}</span>
            </NavLink>
          )}
        </span>
      </nav>
      <main>
        <Outlet />
      </main>
      <CommandPalette isOpen={isPaletteOpen} onClose={() => setIsPaletteOpen(false)} />
    </div>
  )
}

