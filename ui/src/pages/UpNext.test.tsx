import { describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, delay, http } from 'msw'
import { api, server } from '../test/server'
import { renderApp } from '../test/renderApp'

const me = { id: 1, username: 'david' }

function authed() {
  server.use(http.get(api('/api/auth/me'), () => HttpResponse.json(me)))
}

const show = { id: 10, tmdbId: 1399, title: 'Game of Thrones', posterPath: 'tv/1399.jpg', status: 'Returning Series' }
const ep1 = { id: 100, showId: 10, season: 1, number: 1, title: 'Winter Is Coming', airDate: '2011-04-17' }
const ep2 = { id: 101, showId: 10, season: 1, number: 2, title: 'The Kingsroad', airDate: '2011-04-24' }

describe('Up Next dashboard', () => {
  it('renders guidance text when there are no shows to watch', async () => {
    authed()
    server.use(http.get(api('/api/tv/up-next'), () => HttpResponse.json({ items: [] })))

    renderApp('/')

    expect(await screen.findByText(/nothing watched in the last two weeks/i)).toBeInTheDocument()
    // Empty state, never a blank screen: the add-a-show flow is still offered.
    expect(screen.getByRole('search')).toBeInTheDocument()
  })

  it('marks an episode watched optimistically and swaps in the next episode on success', async () => {
    authed()
    server.use(
      http.get(api('/api/tv/up-next'), () =>
        HttpResponse.json({ items: [{ show, episode: ep1 }] }),
      ),
      http.post(api('/api/tv/episodes/100/watch'), async () => {
        // Hold the response so the optimistic (pending) state is observable
        // before the success swap runs.
        await delay(50)
        return HttpResponse.json({ nextUp: ep2 })
      }),
    )

    renderApp('/')
    const user = userEvent.setup()

    const card = (await screen.findByRole('heading', { name: 'Game of Thrones' })).closest('li')!
    expect(within(card).getByText(/S01E01/)).toBeInTheDocument()

    const button = within(card).getByRole('button', { name: /mark game of thrones s01e01 watched/i })
    await user.click(button)

    // Optimistic: the checkmark reflects the pending watched state immediately.
    await waitFor(() => expect(button).toHaveAttribute('aria-pressed', 'true'))

    // On success the card swaps to the next episode in place.
    expect(await screen.findByText(/S01E02/)).toBeInTheDocument()
    expect(screen.queryByText(/S01E01/)).not.toBeInTheDocument()
  })

  it('rolls back the optimistic checkmark when the watch request fails', async () => {
    authed()
    server.use(
      http.get(api('/api/tv/up-next'), () =>
        HttpResponse.json({ items: [{ show, episode: ep1 }] }),
      ),
      http.post(api('/api/tv/episodes/100/watch'), () =>
        HttpResponse.json({ error: 'internal', message: 'boom' }, { status: 500 }),
      ),
    )

    renderApp('/')
    const user = userEvent.setup()

    const button = await screen.findByRole('button', {
      name: /mark game of thrones s01e01 watched/i,
    })
    await user.click(button)

    // After the error the optimistic state rolls back and the episode remains.
    await waitFor(() => expect(button).toHaveAttribute('aria-pressed', 'false'))
    expect(screen.getByText(/S01E01/)).toBeInTheDocument()
    expect(await screen.findByText(/could not mark the episode watched/i)).toBeInTheDocument()
  })
})

describe('show search and add', () => {
  it('searches and adds a show', async () => {
    authed()
    server.use(
      http.get(api('/api/tv/up-next'), () => HttpResponse.json({ items: [] })),
      http.get(api('/api/tv/search'), () =>
        HttpResponse.json({
          results: [
            { id: 1399, name: 'Game of Thrones', overview: 'Nine noble families.', firstAirDate: '2011-04-17', posterPath: 'tv/1399.jpg' },
          ],
        }),
      ),
      http.post(api('/api/tv/shows'), () =>
        HttpResponse.json({ show, item: { id: 1, type: 'TV', externalId: '1399', title: 'Game of Thrones', status: 'WATCHING' } }, { status: 201 }),
      ),
    )

    renderApp('/')
    const user = userEvent.setup()

    await user.type(await screen.findByLabelText(/search tv shows/i), 'thrones')
    await user.click(screen.getByRole('button', { name: /^search$/i }))

    await user.click(await screen.findByRole('button', { name: /add game of thrones/i }))
    expect(await screen.findByText(/^added$/i)).toBeInTheDocument()
  })

  it('handles a duplicate add (409) gracefully', async () => {
    authed()
    server.use(
      http.get(api('/api/tv/up-next'), () => HttpResponse.json({ items: [] })),
      http.get(api('/api/tv/search'), () =>
        HttpResponse.json({
          results: [
            { id: 1399, name: 'Game of Thrones', overview: '', firstAirDate: '2011-04-17', posterPath: '' },
          ],
        }),
      ),
      http.post(api('/api/tv/shows'), () =>
        HttpResponse.json(
          { error: 'duplicate_item', message: 'already tracked', item: { id: 1, type: 'TV', externalId: '1399', title: 'Game of Thrones', status: 'WATCHING' } },
          { status: 409 },
        ),
      ),
    )

    renderApp('/')
    const user = userEvent.setup()

    await user.type(await screen.findByLabelText(/search tv shows/i), 'thrones')
    await user.click(screen.getByRole('button', { name: /^search$/i }))
    await user.click(await screen.findByRole('button', { name: /add game of thrones/i }))

    expect(await screen.findByText(/already in your library/i)).toBeInTheDocument()
  })
})
