import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from '../test/server'
import { renderApp } from '../test/renderApp'

const me = { id: 1, username: 'david' }

function authed() {
  server.use(http.get(api('/api/auth/me'), () => HttpResponse.json(me)))
}

const tvItem = {
  id: 1,
  type: 'TV',
  externalId: '1399',
  title: 'Game of Thrones',
  status: 'WATCHING',
  progress: 0,
  updatedAt: '2026-07-01T00:00:00Z',
}
const bookItem = {
  id: 2,
  type: 'BOOK',
  externalId: '9780553103540',
  title: 'A Game of Thrones',
  status: 'READING',
  progress: 100,
  updatedAt: '2026-07-01T00:00:00Z',
}

describe('Library page', () => {
  it('lists items and renders empty-state guidance when there are none', async () => {
    authed()
    server.use(http.get(api('/api/library'), () => HttpResponse.json([])))

    renderApp('/library')

    expect(await screen.findByText(/no items match these filters/i)).toBeInTheDocument()
  })

  it('updates an item status inline', async () => {
    authed()
    let patched: unknown = null
    let status = 'WATCHING'
    server.use(
      // The refetch after a successful PATCH must reflect the new status.
      http.get(api('/api/library'), () => HttpResponse.json([{ ...tvItem, status }])),
      http.patch(api('/api/items/1'), async ({ request }) => {
        patched = await request.json()
        status = 'COMPLETED'
        return HttpResponse.json({ ...tvItem, status })
      }),
    )

    renderApp('/library')
    const user = userEvent.setup()

    const select = await screen.findByLabelText(/status for game of thrones/i)
    await user.selectOptions(select, 'COMPLETED')

    await screen.findByRole('option', { name: 'COMPLETED', selected: true })
    expect(patched).toEqual({ status: 'COMPLETED' })
  })

  it('edits book progress on blur', async () => {
    authed()
    let patched: unknown = null
    server.use(
      http.get(api('/api/library'), () => HttpResponse.json([bookItem])),
      http.patch(api('/api/items/2'), async ({ request }) => {
        patched = await request.json()
        return HttpResponse.json({ ...bookItem, progress: 150 })
      }),
    )

    renderApp('/library')
    const user = userEvent.setup()

    const page = await screen.findByLabelText(/page for a game of thrones/i)
    await user.clear(page)
    await user.type(page, '150')
    await user.tab()

    await screen.findByDisplayValue('150')
    expect(patched).toEqual({ progress: 150 })
  })

  it('deletes an item only after confirmation', async () => {
    authed()
    let deleted = false
    server.use(
      http.get(api('/api/library'), () =>
        HttpResponse.json(deleted ? [] : [tvItem]),
      ),
      http.delete(api('/api/items/1'), () => {
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )

    renderApp('/library')
    const user = userEvent.setup()

    const row = (await screen.findByText('Game of Thrones')).closest('li')!
    await user.click(within(row).getByRole('button', { name: /delete game of thrones/i }))
    // Confirm step appears; nothing deleted yet.
    expect(deleted).toBe(false)
    await user.click(within(row).getByRole('button', { name: /^confirm$/i }))

    expect(await screen.findByText(/no items match these filters/i)).toBeInTheDocument()
  })
})
