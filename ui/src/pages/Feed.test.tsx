import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from '../test/server'
import { renderApp } from '../test/renderApp'

const me = { id: 1, username: 'david' }

function entry(id: number, action: string) {
  return {
    user: { id: 1, username: 'david' },
    action,
    media: { type: 'TV', title: `Show ${id}`, id: String(id) },
    // Descending timestamps so ordering is stable and unambiguous.
    timestamp: new Date(Date.UTC(2026, 0, 100 - id)).toISOString(),
  }
}

describe('Feed pagination', () => {
  it('appends the second page without duplicating entries, passing nextBefore back verbatim', async () => {
    const cursor = 'CURSOR-PAGE-1'
    const requestedBefores: (string | null)[] = []

    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.get(api('/api/users'), () => HttpResponse.json([])),
      http.get(api('/api/feed'), ({ request }) => {
        const url = new URL(request.url)
        const before = url.searchParams.get('before')
        requestedBefores.push(before)
        if (before === null) {
          return HttpResponse.json({
            entries: [entry(1, 'watched S01E01 of Show 1'), entry(2, 'watched S01E01 of Show 2')],
            nextBefore: cursor,
          })
        }
        // Second page only served for the exact cursor returned by page 1.
        expect(before).toBe(cursor)
        return HttpResponse.json({
          entries: [entry(3, 'watched S01E01 of Show 3'), entry(4, 'watched S01E01 of Show 4')],
          nextBefore: null,
        })
      }),
    )

    renderApp('/feed')
    const user = userEvent.setup()

    // Page 1 rendered.
    expect(await screen.findByText('watched S01E01 of Show 1')).toBeInTheDocument()
    expect(screen.getByText('watched S01E01 of Show 2')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /load more/i }))

    // Page 2 appended.
    expect(await screen.findByText('watched S01E01 of Show 3')).toBeInTheDocument()
    expect(screen.getByText('watched S01E01 of Show 4')).toBeInTheDocument()

    // No duplicates: each action string appears exactly once.
    const list = screen.getAllByRole('list')[0]
    for (const n of [1, 2, 3, 4]) {
      expect(within(list).getAllByText(`watched S01E01 of Show ${n}`)).toHaveLength(1)
    }

    // The second request reused page 1's opaque cursor verbatim.
    expect(requestedBefores).toEqual([null, cursor])

    // Last page → no more "Load more".
    expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument()
  })

  it('shows empty-state guidance when there is no activity', async () => {
    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.get(api('/api/users'), () => HttpResponse.json([])),
      http.get(api('/api/feed'), () => HttpResponse.json({ entries: [], nextBefore: null })),
    )

    renderApp('/feed')

    expect(await screen.findByText(/nothing here yet/i)).toBeInTheDocument()
  })
})
