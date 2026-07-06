import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../api/client'
import { login, register } from '../api/auth'
import { useAuth } from '../hooks/useAuth'

type Mode = 'login' | 'register'

const fieldStyle = { display: 'block', margin: '0.5rem auto', padding: '0.5rem', width: '16rem' }

export default function Login() {
  const { refresh } = useAuth()
  const [mode, setMode] = useState<Mode>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const validate = (): string | null => {
    if (username.trim() === '') return 'Username is required'
    if (password === '') return 'Password is required'
    if (mode === 'register' && inviteCode.trim() === '') return 'Invite code is required'
    return null
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError(null)

    const validationError = validate()
    if (validationError !== null) {
      setError(validationError)
      return
    }

    setSubmitting(true)
    try {
      if (mode === 'register') {
        await register(username.trim(), password, inviteCode.trim())
      }
      await login(username.trim(), password)
      // Refreshing the auth query flips the route guard, which navigates away
      // from /login once the user is set.
      await refresh()
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Something went wrong. Please try again.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  const switchMode = (next: Mode) => {
    setMode(next)
    setError(null)
  }

  return (
    <main style={{ fontFamily: 'system-ui, sans-serif', textAlign: 'center', paddingTop: '4rem' }}>
      <h1>OmniShelf</h1>
      <h2>{mode === 'login' ? 'Sign in' : 'Create account'}</h2>
      <form onSubmit={handleSubmit} noValidate>
        <input
          style={fieldStyle}
          type="text"
          placeholder="Username"
          aria-label="Username"
          autoComplete="username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <input
          style={fieldStyle}
          type="password"
          placeholder="Password"
          aria-label="Password"
          autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {mode === 'register' && (
          <input
            style={fieldStyle}
            type="text"
            placeholder="Invite code"
            aria-label="Invite code"
            value={inviteCode}
            onChange={(e) => setInviteCode(e.target.value)}
          />
        )}
        {error !== null && (
          <p role="alert" style={{ color: 'crimson' }}>
            {error}
          </p>
        )}
        <button type="submit" disabled={submitting}>
          {mode === 'login' ? 'Sign in' : 'Register'}
        </button>
      </form>
      <p>
        {mode === 'login' ? (
          <button type="button" onClick={() => switchMode('register')}>
            Have an invite code? Register
          </button>
        ) : (
          <button type="button" onClick={() => switchMode('login')}>
            Already have an account? Sign in
          </button>
        )}
      </p>
    </main>
  )
}
