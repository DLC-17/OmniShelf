import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from './server'
import { renderApp } from './renderApp'

const me = { id: 1, username: 'david' }

describe('library export in settings', () => {
  it('renders export options and triggers download for JSON and CSV', async () => {
    let jsonRequested = false
    let csvRequested = false

    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.get(api('/api/stats/time-spent'), () =>
        HttpResponse.json({ minutesTv: 0, minutesMovie: 0, minutesBook: 0, minutesGame: 0 }),
      ),
      http.get(api('/api/stats/heatmap'), () => HttpResponse.json([])),
      http.get(api('/api/stats/badges'), () => HttpResponse.json([])),
      http.get(api('/api/users/export'), ({ request }) => {
        const url = new URL(request.url)
        const format = url.searchParams.get('format')
        if (format === 'csv') {
          csvRequested = true
          return new HttpResponse('id,type,title\n1,TV,Breaking Bad', {
            headers: { 'Content-Type': 'text/csv' },
          })
        }
        jsonRequested = true
        return HttpResponse.json([
          { id: 1, type: 'TV', title: 'Breaking Bad', status: 'COMPLETED' },
        ])
      }),
    )

    // Mock URL.createObjectURL and URL.revokeObjectURL
    window.URL.createObjectURL = vi.fn(() => 'blob:mock-url')
    window.URL.revokeObjectURL = vi.fn()

    renderApp('/settings')
    const user = userEvent.setup()

    expect(await screen.findByRole('heading', { name: /export library data/i })).toBeInTheDocument()

    const jsonBtn = screen.getByRole('button', { name: /download json/i })
    await user.click(jsonBtn)
    expect(jsonRequested).toBe(true)

    const csvBtn = screen.getByRole('button', { name: /download csv/i })
    await user.click(csvBtn)
    expect(csvRequested).toBe(true)
  })
})
