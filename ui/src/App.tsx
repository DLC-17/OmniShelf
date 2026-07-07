import { useEffect } from 'react'
import {
  BrowserRouter,
  Navigate,
  Outlet,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from 'react-router-dom'
import { setUnauthorizedHandler } from './api/client'
import Layout from './components/Layout'
import { useAuth } from './hooks/useAuth'
import Discover from './pages/Discover'
import Feed from './pages/Feed'
import Import from './pages/Import'
import Library from './pages/Library'
import Login from './pages/Login'
import Scan from './pages/Scan'
import Settings from './pages/Settings'
import UpNext from './pages/UpNext'
import UserLibrary from './pages/UserLibrary'

/**
 * Global 401 handling (E12): any API response with status 401 clears local
 * auth state and lands the user on /login — unless they are already there.
 */
function UnauthorizedRedirect() {
  const navigate = useNavigate()
  const location = useLocation()
  const { clear } = useAuth()

  useEffect(() => {
    setUnauthorizedHandler(() => {
      clear()
      if (location.pathname !== '/login') {
        navigate('/login', { replace: true })
      }
    })
    return () => setUnauthorizedHandler(null)
  }, [clear, location.pathname, navigate])

  return null
}

function RequireAuth() {
  const { user, isLoading } = useAuth()
  if (isLoading) {
    return <p className="page-loading">Loading…</p>
  }
  if (user === null) {
    return <Navigate to="/login" replace />
  }
  return <Outlet />
}

/** Authenticated users have no business on /login — send them home. */
function RedirectIfAuthed() {
  const { user, isLoading } = useAuth()
  if (isLoading) {
    return <p className="page-loading">Loading…</p>
  }
  if (user !== null) {
    return <Navigate to="/" replace />
  }
  return <Outlet />
}

/** Route tree without a router, so tests can mount it inside a MemoryRouter. */
export function AppRoutes() {
  return (
    <>
      <UnauthorizedRedirect />
      <Routes>
        <Route element={<RedirectIfAuthed />}>
          <Route path="/login" element={<Login />} />
        </Route>
        <Route element={<RequireAuth />}>
          <Route element={<Layout />}>
            <Route path="/" element={<UpNext />} />
            <Route path="/discover" element={<Discover />} />
            <Route path="/library" element={<Library />} />
            <Route path="/scan" element={<Scan />} />
            <Route path="/feed" element={<Feed />} />
            <Route path="/import" element={<Import />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/users/:id" element={<UserLibrary />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  )
}
