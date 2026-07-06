import { describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from '../test/server'
import { renderApp } from '../test/renderApp'

const me = { id: 1, username: 'david' }

async function uploadAFile() {
  const user = userEvent.setup()
  const input = await screen.findByLabelText(/export files/i)
  const file = new File(['show,year\nBreaking Bad,2008\n'], 'followed_shows.csv', {
    type: 'text/csv',
  })
  await user.upload(input, file)
  await user.click(screen.getByRole('button', { name: /start import/i }))
  return user
}

describe('Import wizard', () => {
  it('uploads, polls to completion, and posts unresolved titles to the resolve endpoint', async () => {
    let statusCalls = 0
    let resolvePayload: unknown = null

    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.post(api('/api/tv/import'), () =>
        HttpResponse.json({ jobId: 42 }, { status: 202 }),
      ),
      http.get(api('/api/tv/import/42'), () => {
        statusCalls += 1
        if (statusCalls === 1) {
          return HttpResponse.json({
            jobId: 42,
            status: 'RUNNING',
            processed: 1,
            total: 2,
            skipped: 0,
            unresolved: [],
          })
        }
        return HttpResponse.json({
          jobId: 42,
          status: 'DONE',
          processed: 2,
          total: 2,
          skipped: 0,
          unresolved: ['The Wire'],
        })
      }),
      http.post(api('/api/tv/import/42/resolve'), async ({ request }) => {
        resolvePayload = await request.json()
        return HttpResponse.json({
          jobId: 42,
          status: 'DONE',
          processed: 3,
          total: 3,
          skipped: 0,
          unresolved: [],
        })
      }),
    )

    renderApp('/import')
    const user = await uploadAFile()

    // Polls to completion → resolution form for the unmatched title.
    const form = await screen.findByRole(
      'form',
      { name: /resolve unmatched titles/i },
      { timeout: 4000 },
    )
    expect(within(form).getByText('The Wire')).toBeInTheDocument()

    await user.type(within(form).getByPlaceholderText(/tmdb id/i), '1438')
    await user.click(within(form).getByRole('button', { name: /resolve titles/i }))

    expect(await screen.findByText(/everything matched/i)).toBeInTheDocument()
    expect(resolvePayload).toEqual({ mappings: { 'The Wire': 1438 } })
  })

  it('surfaces a retry message when the server is busy (503 jobs_busy)', async () => {
    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.post(api('/api/tv/import'), () =>
        HttpResponse.json(
          { error: 'jobs_busy', message: 'a sync or import is running; retry shortly' },
          { status: 503, headers: { 'Retry-After': '10' } },
        ),
      ),
    )

    renderApp('/import')
    await uploadAFile()

    expect(await screen.findByRole('alert')).toHaveTextContent(/already running/i)
  })

  it('shows the error when the job FAILED', async () => {
    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.post(api('/api/tv/import'), () => HttpResponse.json({ jobId: 7 }, { status: 202 })),
      http.get(api('/api/tv/import/7'), () =>
        HttpResponse.json({
          jobId: 7,
          status: 'FAILED',
          processed: 0,
          total: 0,
          skipped: 0,
          unresolved: [],
          error: 'interrupted',
        }),
      ),
    )

    renderApp('/import')
    await uploadAFile()

    expect(await screen.findByText(/import failed: interrupted/i)).toBeInTheDocument()
  })
})
