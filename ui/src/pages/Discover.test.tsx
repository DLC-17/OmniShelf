import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from '../test/server'
import { renderApp } from '../test/renderApp'

const me = { id: 1, username: 'david' }

function authed() {
  server.use(http.get(api('/api/auth/me'), () => HttpResponse.json(me)))
}

const suggestion = {
  tmdbId: 1396,
  title: 'Breaking Bad',
  overview: 'A chemistry teacher turns to crime.',
  posterPath: '/bb.jpg',
  firstAirDate: '2008-01-20',
  suggestedBy: 'Game of Thrones',
}

describe('Discover page', () => {
  it('shows suggestions tagged with their source and empty-state when none', async () => {
    authed()
    server.use(http.get(api('/api/tv/discover'), () => HttpResponse.json({ items: [] })))
    renderApp('/discover')
    expect(await screen.findByText(/no suggestions yet/i)).toBeInTheDocument()
  })

  it('adds a suggestion and removes its card', async () => {
    authed()
    let added: number | null = null
    server.use(
      http.get(api('/api/tv/discover'), () => HttpResponse.json({ items: [suggestion] })),
      http.post(api('/api/tv/shows'), async ({ request }) => {
        added = ((await request.json()) as { tmdbId: number }).tmdbId
        return HttpResponse.json({ show: {}, item: {} }, { status: 201 })
      }),
    )

    renderApp('/discover')
    const user = userEvent.setup()

    expect(await screen.findByText(/suggested based on game of thrones/i)).toBeInTheDocument()
    await user.click(await screen.findByRole('button', { name: /add breaking bad/i }))

    expect(added).toBe(1396)
    await waitForRemoved('Breaking Bad')
  })

  it('rejects a suggestion and removes its card', async () => {
    authed()
    let rejected: number | null = null
    server.use(
      http.get(api('/api/tv/discover'), () => HttpResponse.json({ items: [suggestion] })),
      http.post(api('/api/tv/discover/reject'), async ({ request }) => {
        rejected = ((await request.json()) as { tmdbId: number }).tmdbId
        return new HttpResponse(null, { status: 204 })
      }),
    )

    renderApp('/discover')
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /reject breaking bad/i }))
    expect(rejected).toBe(1396)
    await waitForRemoved('Breaking Bad')
  })
})

async function waitForRemoved(text: string) {
  const { waitForElementToBeRemoved } = await import('@testing-library/react')
  const el = screen.queryByText(text)
  if (el !== null) await waitForElementToBeRemoved(() => screen.queryByText(text))
}
