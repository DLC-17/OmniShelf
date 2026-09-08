import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from './server'
import { renderApp } from './renderApp'

describe('forced password change modal', () => {
  it('renders blocking modal when mustChangePassword is true', async () => {
    server.use(
      http.get(api('/api/auth/me'), () =>
        HttpResponse.json({ id: 1, username: 'bob', isAdmin: false, mustChangePassword: true })
      )
    )

    renderApp('/')

    expect(await screen.findByRole('heading', { name: /password change required/i })).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/minimum 8 characters/i)).toBeInTheDocument()
  })

  it('submits new password and updates user state', async () => {
    let changed = false
    server.use(
      http.get(api('/api/auth/me'), () =>
        HttpResponse.json({
          id: 1,
          username: 'bob',
          isAdmin: false,
          mustChangePassword: !changed,
        })
      ),
      http.post(api('/api/auth/change-password'), async () => {
        changed = true
        return HttpResponse.json({
          id: 1,
          username: 'bob',
          isAdmin: false,
          mustChangePassword: false,
        })
      })
    )

    renderApp('/')
    const user = userEvent.setup()

    const pwdInput = await screen.findByPlaceholderText(/minimum 8 characters/i)
    const confirmInput = screen.getByPlaceholderText(/re-enter new password/i)

    await user.type(pwdInput, 'myNewSuperPassword123!')
    await user.type(confirmInput, 'myNewSuperPassword123!')
    await user.click(screen.getByRole('button', { name: /set new password/i }))

    expect(await screen.findByRole('heading', { name: /up next/i })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /password change required/i })).not.toBeInTheDocument()
  })
})

describe('admin user management panel in Settings', () => {
  it('renders admin card when user is admin', async () => {
    server.use(
      http.get(api('/api/auth/me'), () =>
        HttpResponse.json({ id: 1, username: 'admin_user', isAdmin: true, mustChangePassword: false })
      ),
      http.get(api('/api/admin/users'), () =>
        HttpResponse.json([
          { id: 1, username: 'admin_user', isAdmin: true, mustChangePassword: false, createdAt: '2026-01-01T00:00:00Z' },
          { id: 2, username: 'regular_user', isAdmin: false, mustChangePassword: true, createdAt: '2026-01-02T00:00:00Z' },
        ])
      )
    )

    renderApp('/settings')

    expect(await screen.findByRole('heading', { name: /admin: user management/i })).toBeInTheDocument()
    expect(screen.getByText('regular_user')).toBeInTheDocument()
    expect(screen.getByText(/password reset pending/i)).toBeInTheDocument()
  })

  it('does not render admin card for regular users', async () => {
    server.use(
      http.get(api('/api/auth/me'), () =>
        HttpResponse.json({ id: 2, username: 'regular_user', isAdmin: false, mustChangePassword: false })
      )
    )

    renderApp('/settings')

    expect(await screen.findByRole('heading', { name: /profile & settings/i })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /admin: user management/i })).not.toBeInTheDocument()
  })
})
