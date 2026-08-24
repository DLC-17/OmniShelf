import { describe, expect, it } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from './server'
import { renderApp } from './renderApp'

const me = { id: 1, username: 'david' }

describe('command palette', () => {
  it('opens on clicking the topnav search button and closes on Esc', async () => {
    server.use(http.get(api('/api/auth/me'), () => HttpResponse.json(me)))

    renderApp('/')
    const user = userEvent.setup()

    expect(await screen.findByRole('heading', { name: /up next/i })).toBeInTheDocument()

    // Command palette is not visible initially
    expect(screen.queryByRole('dialog', { name: /command palette/i })).not.toBeInTheDocument()

    // Click topnav search button
    const searchBtn = screen.getByRole('button', { name: /search and command palette/i })
    await user.click(searchBtn)

    // Command palette is now visible
    expect(await screen.findByRole('dialog', { name: /command palette/i })).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/type a command/i)).toBeInTheDocument()

    // Press Escape to close
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(screen.queryByRole('dialog', { name: /command palette/i })).not.toBeInTheDocument()
  })

  it('filters actions and navigates when an action is selected', async () => {
    server.use(http.get(api('/api/auth/me'), () => HttpResponse.json(me)))

    renderApp('/')
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /search and command palette/i }))
    const input = await screen.findByPlaceholderText(/type a command/i)

    // Type "barcode" to find the Barcode Scanner action
    await user.type(input, 'barcode')
    expect(screen.getByText(/open barcode scanner/i)).toBeInTheDocument()

    // Click the action
    await user.click(screen.getByText(/open barcode scanner/i))

    // Should navigate to Scan page
    expect(await screen.findByRole('heading', { name: /^scan$/i })).toBeInTheDocument()
  })
})
