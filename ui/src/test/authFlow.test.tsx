import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from './server'
import { renderApp } from './renderApp'

const unauthorized = () =>
  HttpResponse.json({ error: 'unauthorized', message: 'Sign in required' }, { status: 401 })

const me = { id: 1, username: 'david' }

describe('auth routing', () => {
  it('redirects to /login when /api/auth/me returns 401', async () => {
    server.use(http.get(api('/api/auth/me'), unauthorized))

    renderApp('/')

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /up next/i })).not.toBeInTheDocument()
  })

  it('redirects an authenticated user away from /login', async () => {
    server.use(http.get(api('/api/auth/me'), () => HttpResponse.json(me)))

    renderApp('/login')

    expect(await screen.findByRole('heading', { name: /up next/i })).toBeInTheDocument()
  })
})

describe('login form', () => {
  it('signs in and navigates to Up Next on success', async () => {
    let authenticated = false
    server.use(
      http.get(api('/api/auth/me'), () =>
        authenticated ? HttpResponse.json(me) : unauthorized(),
      ),
      http.post(api('/api/auth/login'), () => {
        authenticated = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    renderApp('/login')
    const user = userEvent.setup()

    await user.type(await screen.findByLabelText(/username/i), 'david')
    await user.type(screen.getByLabelText(/password/i), 'hunter22')
    await user.click(screen.getByRole('button', { name: /^sign in$/i }))

    expect(await screen.findByRole('heading', { name: /up next/i })).toBeInTheDocument()
    expect(screen.getByText('david')).toBeInTheDocument()
  })

  it('shows the envelope message when login fails', async () => {
    server.use(
      http.get(api('/api/auth/me'), unauthorized),
      http.post(api('/api/auth/login'), () =>
        HttpResponse.json(
          { error: 'invalid_credentials', message: 'Wrong username or password' },
          { status: 401 },
        ),
      ),
    )

    renderApp('/login')
    const user = userEvent.setup()

    await user.type(await screen.findByLabelText(/username/i), 'david')
    await user.type(screen.getByLabelText(/password/i), 'nope')
    await user.click(screen.getByRole('button', { name: /^sign in$/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Wrong username or password')
    // Still on the login page — no redirect on a login-page 401.
    expect(screen.getByRole('heading', { name: /sign in/i })).toBeInTheDocument()
  })
})

describe('register form', () => {
  it('requires an invite code before submitting', async () => {
    // Only auth/me is stubbed: a register/login request would fail the test
    // via onUnhandledRequest: 'error'.
    server.use(http.get(api('/api/auth/me'), unauthorized))

    renderApp('/login')
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /register/i }))
    await user.type(screen.getByLabelText(/username/i), 'newuser')
    await user.type(screen.getByLabelText(/password/i), 'hunter22')
    await user.click(screen.getByRole('button', { name: /^register$/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/invite code is required/i)
  })

  it('requires username and password', async () => {
    server.use(http.get(api('/api/auth/me'), unauthorized))

    renderApp('/login')
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /register/i }))
    await user.click(screen.getByRole('button', { name: /^register$/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/username is required/i)
  })

  it('registers, logs in and lands on Up Next', async () => {
    let authenticated = false
    server.use(
      http.get(api('/api/auth/me'), () =>
        authenticated ? HttpResponse.json({ id: 2, username: 'newuser' }) : unauthorized(),
      ),
      http.post(api('/api/auth/register'), () => new HttpResponse(null, { status: 201 })),
      http.post(api('/api/auth/login'), () => {
        authenticated = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    renderApp('/login')
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /register/i }))
    await user.type(screen.getByLabelText(/username/i), 'newuser')
    await user.type(screen.getByLabelText(/password/i), 'hunter22')
    await user.type(screen.getByLabelText(/invite code/i), 'INVITE-123')
    await user.click(screen.getByRole('button', { name: /^register$/i }))

    expect(await screen.findByRole('heading', { name: /up next/i })).toBeInTheDocument()
  })
})

describe('global 401 handling', () => {
  it('clears auth and redirects to /login when a later request 401s', async () => {
    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.post(api('/api/auth/logout'), () =>
        HttpResponse.json({ error: 'unauthorized', message: 'Session expired' }, { status: 401 }),
      ),
    )

    renderApp('/')
    const user = userEvent.setup()

    // Sign out now lives on the settings page, reached via the username nav link.
    await user.click(await screen.findByRole('link', { name: /settings/i }))
    // Trigger an API call that 401s (logout stubbed to fail with 401).
    await user.click(await screen.findByRole('button', { name: /sign out/i }))

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
  })
})
