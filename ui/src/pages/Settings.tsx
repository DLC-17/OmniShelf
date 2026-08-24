import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { logout } from '../api/auth'
import { useAuth } from '../hooks/useAuth'
import type { Theme } from '../lib/theme'
import { THEMES } from '../lib/theme'
import { useTheme } from '../theme/ThemeContext'
import { connectGOG, disconnectGOG, getGOGAuthURL, getGOGStatus, syncGOGLibrary } from '../api/gog'
import type { GOGStatus } from '../api/gog'
import Stats from './Stats'

/**
 * Combined Profile & Settings page reached by clicking the user in the navbar.
 * Contains user profile info, statistics (time spent, heatmap, badges),
 * theme customization, GOG sign-in (official GOG page primary, email/password fallback)
 * & DRM-free sync, data import links, 1-click library backup/export, and sign-out.
 */
export default function Settings() {
  const { user, clear } = useAuth()
  const { theme: selectedTheme, setTheme } = useTheme()
  const [exporting, setExporting] = useState<'json' | 'csv' | null>(null)
  const [exportError, setExportError] = useState<string | null>(null)

  // GOG Integration State
  const [gogStatus, setGogStatus] = useState<GOGStatus | null>(null)
  const [gogLoading, setGogLoading] = useState(false)
  const [gogSyncing, setGogSyncing] = useState(false)
  const [gogMessage, setGogMessage] = useState<string | null>(null)
  // Tracks whether the official GOG window is open and awaiting the redirect URL
  const [awaitingGogCode, setAwaitingGogCode] = useState(false)
  const [gogCodeInput, setGogCodeInput] = useState('')

  const popupRef = useRef<Window | null>(null)
  const popupPollRef = useRef<number | null>(null)
  const gogStatusRef = useRef(gogStatus)
  gogStatusRef.current = gogStatus

  useEffect(() => {
    loadGOGStatus()

    // Listen for GOG_CONNECTED message from the email/password popup
    const handleMessage = async (e: MessageEvent) => {
      if (e.data?.type === 'GOG_CONNECTED') {
        cleanup()
        setGogLoading(false)
        setAwaitingGogCode(false)
        setGogMessage(`✓ Connected as ${e.data.username || 'GOG user'}! Syncing DRM-free games…`)
        await loadGOGStatus()
        await runAutoSync()
      }
    }

    window.addEventListener('message', handleMessage)
    return () => {
      window.removeEventListener('message', handleMessage)
      cleanup()
    }
  }, [])

  const cleanup = () => {
    if (popupPollRef.current) {
      window.clearInterval(popupPollRef.current)
      popupPollRef.current = null
    }
    if (popupRef.current && !popupRef.current.closed) {
      popupRef.current.close()
    }
  }

  const loadGOGStatus = async () => {
    try {
      const status = await getGOGStatus()
      setGogStatus(status)
      return status
    } catch {
      setGogStatus({ connected: false, username: '', source: 'none', lastSyncedAt: null })
      return null
    }
  }

  const runAutoSync = async () => {
    setGogSyncing(true)
    try {
      const syncRes = await syncGOGLibrary()
      setGogMessage(syncRes.message)
      await loadGOGStatus()
    } catch (err) {
      setGogMessage(err instanceof Error ? err.message : 'Library sync failed.')
    } finally {
      setGogSyncing(false)
    }
  }

  // ── Option A: Official GOG Page (Google, Discord, etc.) ──────────────
  // Opens the real auth.gog.com in a popup so users can use any SSO method.
  // After GOG login completes, user pastes the redirect URL and OmniShelf
  // auto-extracts the code, exchanges tokens, and syncs.
  const handleOpenOfficialGOGPage = async () => {
    setGogLoading(true)
    setGogMessage(null)
    setAwaitingGogCode(false)
    setGogCodeInput('')

    try {
      const { authUrl } = await getGOGAuthURL()

      const width = 520
      const height = 700
      const left = window.screenX + (window.outerWidth - width) / 2
      const top = window.screenY + (window.outerHeight - height) / 2

      const popup = window.open(
        authUrl,
        'gog_official_signin',
        `width=${width},height=${height},left=${left},top=${top},status=0,menubar=0,toolbar=0`
      )

      popupRef.current = popup
      setAwaitingGogCode(true)
      setGogMessage('Sign into GOG using your preferred method (Google, Discord, email, etc.)…')

      if (popupPollRef.current) {
        window.clearInterval(popupPollRef.current)
      }

      // Poll to detect if user closes the popup without completing
      popupPollRef.current = window.setInterval(() => {
        if (popup && popup.closed) {
          window.clearInterval(popupPollRef.current!)
          popupPollRef.current = null
          // Don't clear awaitingGogCode yet — user might still paste the URL
          // after closing the GOG window (the URL is still in their address bar)
          setGogLoading(false)
        }
      }, 1500)
    } catch (err) {
      setGogLoading(false)
      setAwaitingGogCode(false)
      setGogMessage(err instanceof Error ? err.message : 'Could not open GOG sign-in.')
    }
  }

  // Auto-detect and extract code from pasted URL or raw code
  const handleCodeInputChange = async (value: string) => {
    setGogCodeInput(value)

    // Auto-detect: if the pasted text contains "code=", extract and submit immediately
    if (value.includes('code=')) {
      const code = extractCodeFromInput(value)
      if (code) {
        await submitGogCode(code)
      }
    }
  }

  const extractCodeFromInput = (input: string): string | null => {
    const trimmed = input.trim()
    // Handle full URL: https://embed.gog.com/on_login_success?origin=client&code=XXXXXX
    if (trimmed.includes('code=')) {
      const parts = trimmed.split('code=')
      if (parts.length > 1) {
        return parts[1].split('&')[0].split('#')[0].trim()
      }
    }
    // Handle raw code (no URL)
    if (trimmed.length > 10 && !trimmed.includes(' ') && !trimmed.includes('/')) {
      return trimmed
    }
    return null
  }

  const submitGogCode = async (code: string) => {
    setGogLoading(true)
    setGogMessage('Connecting to GOG…')
    try {
      const res = await connectGOG({ code })
      cleanup()
      setAwaitingGogCode(false)
      setGogCodeInput('')
      setGogMessage(res.message || `✓ Connected as ${res.username || 'GOG user'}!`)
      await loadGOGStatus()
      await runAutoSync()
    } catch (err) {
      setGogMessage(err instanceof Error ? err.message : 'Failed to connect. Please try again.')
    } finally {
      setGogLoading(false)
    }
  }

  const handleManualCodeSubmit = async () => {
    const code = extractCodeFromInput(gogCodeInput)
    if (code) {
      await submitGogCode(code)
    }
  }

  // ── Option B: Email & Password Popup (Fallback) ──────────────────────
  // Opens the OmniShelf-hosted popup that authenticates via GOG's API
  // server-side. Auto-closes and syncs. Email/password only, no SSO.
  const handleOpenEmailPasswordPopup = () => {
    setGogLoading(true)
    setGogMessage('Email & password sign-in window opened…')
    setAwaitingGogCode(false)
    setGogCodeInput('')

    const width = 460
    const height = 580
    const left = window.screenX + (window.outerWidth - width) / 2
    const top = window.screenY + (window.outerHeight - height) / 2

    const popup = window.open(
      '/api/gog/popup',
      'gog_email_signin',
      `width=${width},height=${height},left=${left},top=${top},status=0,menubar=0,toolbar=0`
    )

    popupRef.current = popup

    if (popupPollRef.current) {
      window.clearInterval(popupPollRef.current)
    }

    popupPollRef.current = window.setInterval(async () => {
      if (popup && popup.closed) {
        window.clearInterval(popupPollRef.current!)
        popupPollRef.current = null
        setGogLoading(false)
        const updated = await loadGOGStatus()
        if (updated?.connected && !gogStatusRef.current?.connected) {
          await runAutoSync()
        }
      } else {
        const updated = await loadGOGStatus()
        if (updated?.connected && !gogStatusRef.current?.connected) {
          if (popup && !popup.closed) popup.close()
          window.clearInterval(popupPollRef.current!)
          popupPollRef.current = null
          setGogLoading(false)
          setGogMessage(`✓ Connected as ${updated.username}! Syncing DRM-free games…`)
          await runAutoSync()
        }
      }
    }, 1200)
  }

  const handleSyncGOG = async () => {
    setGogSyncing(true)
    setGogMessage(null)
    try {
      const res = await syncGOGLibrary()
      setGogMessage(res.message)
      await loadGOGStatus()
    } catch (err) {
      setGogMessage(err instanceof Error ? err.message : 'GOG sync failed.')
    } finally {
      setGogSyncing(false)
    }
  }

  const handleDisconnectGOG = async () => {
    setGogLoading(true)
    setGogMessage(null)
    try {
      await disconnectGOG()
      setGogMessage('GOG account disconnected.')
      await loadGOGStatus()
    } catch (err) {
      setGogMessage(err instanceof Error ? err.message : 'Failed to disconnect GOG.')
    } finally {
      setGogLoading(false)
    }
  }

  const handleThemeChange = (newTheme: Theme) => {
    setTheme(newTheme)
  }

  const handleNextTheme = () => {
    const currentIndex = THEMES.findIndex((t) => t.id === selectedTheme)
    const nextIndex = (currentIndex + 1) % THEMES.length
    setTheme(THEMES[nextIndex].id)
  }

  const handlePrevTheme = () => {
    const currentIndex = THEMES.findIndex((t) => t.id === selectedTheme)
    const prevIndex = (currentIndex - 1 + THEMES.length) % THEMES.length
    setTheme(THEMES[prevIndex].id)
  }

  const handleExport = async (format: 'json' | 'csv') => {
    setExporting(format)
    setExportError(null)
    try {
      const res = await fetch(`/api/users/export?format=${format}`, {
        method: 'GET',
        credentials: 'include',
      })
      if (!res.ok) {
        throw new Error(`Export failed with status ${res.status}`)
      }
      const blob = await res.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `omnishelf-export-${new Date().toISOString().split('T')[0]}.${format}`
      document.body.appendChild(a)
      a.click()
      window.URL.revokeObjectURL(url)
      document.body.removeChild(a)
    } catch (err) {
      setExportError(err instanceof Error ? err.message : 'Export failed. Please try again.')
    } finally {
      setExporting(null)
    }
  }

  const handleLogout = async () => {
    try {
      await logout()
    } catch {
      // Drop local state
    }
    clear()
  }

  return (
    <section style={{ maxWidth: '1000px', margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem', flexWrap: 'wrap', gap: '1rem' }}>
        <div>
          <h1>Profile & Settings</h1>
          <p className="muted" style={{ margin: 0 }}>
            Signed in as <strong>{user?.username}</strong>
          </p>
        </div>
        <button type="button" className="btn-danger" onClick={handleLogout}>
          Sign out
        </button>
      </div>

      {/* Embedded Stats Section (Time Spent, Heatmap, Badges) */}
      <Stats />

      {/* Theme Customization Section */}
      <div className="card" style={{ marginTop: '1.5rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.75rem', marginBottom: '1.25rem' }}>
          <div>
            <h2>Theme & Appearance</h2>
            <p className="muted" style={{ margin: 0 }}>
              Choose your preferred visual theme across the application.
            </p>
          </div>
          <div className="theme-cycle-controls" style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
            <button
              type="button"
              className="btn-ghost"
              onClick={handlePrevTheme}
              title="Previous Theme (Cycle)"
              aria-label="Previous Theme"
              style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem', padding: '0.35rem 0.75rem' }}
            >
              ← Previous
            </button>
            <button
              type="button"
              className="btn-ghost"
              onClick={handleNextTheme}
              title="Next Theme (Cycle)"
              aria-label="Next Theme"
              style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem', padding: '0.35rem 0.75rem' }}
            >
              Next →
            </button>
          </div>
        </div>

        <div className="theme-horizontal-row">
          {THEMES.map((theme) => {
            const isActive = selectedTheme === theme.id
            return (
              <button
                key={theme.id}
                type="button"
                className={`theme-card ${isActive ? 'active' : ''}`}
                onClick={() => handleThemeChange(theme.id)}
                aria-pressed={isActive}
              >
                <div className="theme-preview">
                  <span className="theme-swatch" style={{ background: theme.colors.bg }} title="Background" />
                  <span className="theme-swatch" style={{ background: theme.colors.surface }} title="Surface" />
                  <span className="theme-swatch" style={{ background: theme.colors.accent }} title="Accent" />
                  <span className="theme-swatch" style={{ background: theme.colors.text }} title="Text" />
                </div>
                <div className="theme-info">
                  <span className="theme-name">
                    {theme.name} {isActive && '✓'}
                  </span>
                  <span className="theme-desc">{theme.description}</span>
                </div>
              </button>
            )
          })}
        </div>
      </div>

      {/* GOG Integration (DRM-Free Video Games) */}
      <div className="card" style={{ marginTop: '1.5rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.75rem' }}>
          <div>
            <h2 style={{ margin: 0 }}>GOG Sync (DRM-Free Games)</h2>
            <p className="muted" style={{ margin: '0.25rem 0 0' }}>
              Import and sync genuinely owned, DRM-free video games from your GOG account.
            </p>
          </div>
          {gogStatus?.connected ? (
            <span className="badge" style={{ borderColor: 'var(--confirm)', color: 'var(--text)' }}>
              🟢 Connected: {gogStatus.username}
            </span>
          ) : (
            <span className="badge">⚪ Not Connected</span>
          )}
        </div>

        {gogMessage && (
          <p className="muted" style={{ marginTop: '0.75rem', padding: '0.65rem 0.85rem', background: 'var(--surface-alt)', borderRadius: 'var(--radius-sm)' }}>
            {gogMessage}
          </p>
        )}

        {/* Connected state: Sync & Disconnect */}
        {gogStatus?.connected ? (
          <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap', marginTop: '1.25rem', alignItems: 'center' }}>
            <button
              type="button"
              className="btn-primary"
              onClick={handleSyncGOG}
              disabled={gogSyncing}
            >
              {gogSyncing ? '⏳ Syncing Games…' : '🔄 Sync GOG Library Now'}
            </button>
            <button
              type="button"
              className="btn-danger"
              onClick={handleDisconnectGOG}
              disabled={gogLoading}
            >
              Disconnect Account
            </button>
            {gogStatus.lastSyncedAt && (
              <span className="muted" style={{ fontSize: '0.85rem' }}>
                Last synced: {new Date(gogStatus.lastSyncedAt).toLocaleString()}
              </span>
            )}
          </div>
        ) : (
          <>
            {/* Not connected: Sign-in buttons */}
            <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap', marginTop: '1.25rem', alignItems: 'center' }}>
              <button
                type="button"
                className="btn-primary"
                onClick={handleOpenOfficialGOGPage}
                disabled={gogLoading || awaitingGogCode}
              >
                🎮 Sign in with GOG
              </button>
              <button
                type="button"
                className="btn-secondary"
                onClick={handleOpenEmailPasswordPopup}
                disabled={gogLoading}
                title="Sign in with GOG email & password (no Google/Discord)"
              >
                ✉️ Email & Password
              </button>
            </div>

            {/* Code capture bar — appears after opening the official GOG page */}
            {awaitingGogCode && (
              <div style={{
                marginTop: '1.25rem',
                padding: '1rem',
                background: 'var(--surface-alt)',
                borderRadius: 'var(--radius-sm)',
                border: '1px solid var(--accent)',
              }}>
                <p className="muted" style={{ fontSize: '0.88rem', margin: '0 0 0.75rem', lineHeight: 1.5 }}>
                  After signing in on GOG, you'll be redirected to a page with <code style={{ fontSize: '0.82rem' }}>on_login_success</code> in the address bar.
                  Copy and paste the <strong>full address bar URL</strong> below:
                </p>
                <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                  <input
                    type="text"
                    placeholder="Paste the redirect URL here…"
                    value={gogCodeInput}
                    onChange={(e) => handleCodeInputChange(e.target.value)}
                    style={{ flex: 1, minWidth: '260px' }}
                    autoFocus
                  />
                  <button
                    type="button"
                    className="btn-confirm"
                    onClick={handleManualCodeSubmit}
                    disabled={!gogCodeInput.trim() || gogLoading}
                  >
                    Connect
                  </button>
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => { setAwaitingGogCode(false); setGogCodeInput(''); setGogMessage(null); cleanup() }}
                    style={{ padding: '0.5rem 0.75rem' }}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {/* 1-Click Library Backup/Export Section */}
      <div id="export" className="card" style={{ marginTop: '1.5rem' }}>
        <h2>Export Library Data</h2>
        <p className="muted">
          Download a complete backup of all your tracked media (TV shows, movies, books, games, music, cards, progress, and ratings).
        </p>

        {exportError && (
          <p role="alert" className="alert" style={{ marginBottom: '1rem' }}>
            {exportError}
          </p>
        )}

        <div className="export-grid">
          <div className="export-card">
            <h3>JSON Backup</h3>
            <p className="muted">Complete structured dataset with full item metadata, progress, and timestamps.</p>
            <button
              type="button"
              className="btn-primary"
              onClick={() => handleExport('json')}
              disabled={exporting !== null}
            >
              {exporting === 'json' ? 'Exporting JSON…' : '📥 Download JSON'}
            </button>
          </div>

          <div className="export-card">
            <h3>CSV Spreadsheet</h3>
            <p className="muted">Tabular spreadsheet format compatible with Excel, Google Sheets, and Notion.</p>
            <button
              type="button"
              className="btn-confirm"
              onClick={() => handleExport('csv')}
              disabled={exporting !== null}
            >
              {exporting === 'csv' ? 'Exporting CSV…' : '📊 Download CSV'}
            </button>
          </div>
        </div>
      </div>

      {/* Data Import Section */}
      <div className="card" style={{ marginTop: '1.5rem' }}>
        <h2>Import Data</h2>
        <p className="muted">Bring in your history from TV Time or Goodreads (CSV or zip export).</p>
        <Link to="/import" className="btn-primary" role="button" style={{ display: 'inline-block', textDecoration: 'none' }}>
          Import data
        </Link>
      </div>
    </section>
  )
}
