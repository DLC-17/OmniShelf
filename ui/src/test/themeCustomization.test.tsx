import { beforeEach, describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from './server'
import { renderApp } from './renderApp'
import { applyTheme, getStoredTheme, initTheme } from '../lib/theme'

const me = { id: 1, username: 'david' }

describe('theme customization', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })

  it('defaults to dark-espresso', () => {
    expect(getStoredTheme()).toBe('dark-espresso')
    initTheme()
    expect(document.documentElement.dataset.theme).toBe('dark-espresso')
  })

  it('applies and persists theme changes', () => {
    applyTheme('midnight-oled')
    expect(document.documentElement.dataset.theme).toBe('midnight-oled')
    expect(getStoredTheme()).toBe('midnight-oled')

    applyTheme('cream-paper')
    expect(document.documentElement.dataset.theme).toBe('cream-paper')
    expect(getStoredTheme()).toBe('cream-paper')
  })

  it('allows switching themes from the Settings page', async () => {
    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.get(api('/api/stats/time-spent'), () =>
        HttpResponse.json({ minutesTv: 0, minutesMovie: 0, minutesBook: 0, minutesGame: 0 }),
      ),
      http.get(api('/api/stats/heatmap'), () => HttpResponse.json([])),
      http.get(api('/api/stats/badges'), () => HttpResponse.json([])),
    )

    renderApp('/settings')
    const user = userEvent.setup()

    expect(await screen.findByRole('heading', { name: /theme & appearance/i })).toBeInTheDocument()

    // Click Midnight OLED theme card
    const oledCard = screen.getByRole('button', { name: /midnight oled/i })
    await user.click(oledCard)

    expect(document.documentElement.dataset.theme).toBe('midnight-oled')
    expect(getStoredTheme()).toBe('midnight-oled')

    // Click Cream Paper theme card
    const creamCard = screen.getByRole('button', { name: /cream paper/i })
    await user.click(creamCard)

    expect(document.documentElement.dataset.theme).toBe('cream-paper')
    expect(getStoredTheme()).toBe('cream-paper')
  })

  it('allows cycling themes with next/previous buttons', async () => {
    server.use(
      http.get(api('/api/auth/me'), () => HttpResponse.json(me)),
      http.get(api('/api/stats/time-spent'), () =>
        HttpResponse.json({ minutesTv: 0, minutesMovie: 0, minutesBook: 0, minutesGame: 0 }),
      ),
      http.get(api('/api/stats/heatmap'), () => HttpResponse.json([])),
      http.get(api('/api/stats/badges'), () => HttpResponse.json([])),
      http.patch(api('/api/user/theme'), () => HttpResponse.json({ theme: 'midnight-oled' })),
    )

    renderApp('/settings')
    const user = userEvent.setup()

    expect(await screen.findByRole('heading', { name: /theme & appearance/i })).toBeInTheDocument()

    // Cycle Next (from dark-espresso -> midnight-oled)
    const nextBtn = screen.getByRole('button', { name: /next theme/i })
    await user.click(nextBtn)
    expect(document.documentElement.dataset.theme).toBe('midnight-oled')

    // Cycle Prev (from midnight-oled -> dark-espresso)
    const prevBtn = screen.getByRole('button', { name: /previous theme/i })
    await user.click(prevBtn)
    expect(document.documentElement.dataset.theme).toBe('dark-espresso')
  })
})
