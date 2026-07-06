import { afterEach, describe, expect, it, vi } from 'vitest'
import { HttpResponse, http } from 'msw'
import { ApiError, request, setUnauthorizedHandler } from './client'
import { api, server } from '../test/server'

afterEach(() => setUnauthorizedHandler(null))

describe('request', () => {
  it('parses a JSON success body', async () => {
    server.use(
      http.get(api('/api/things'), () => HttpResponse.json({ id: 7, name: 'thing' })),
    )

    const result = await request<{ id: number; name: string }>('/api/things')
    expect(result).toEqual({ id: 7, name: 'thing' })
  })

  it('returns undefined for an empty success body', async () => {
    server.use(http.post(api('/api/auth/logout'), () => new HttpResponse(null, { status: 204 })))

    await expect(request<void>('/api/auth/logout', { method: 'POST' })).resolves.toBeUndefined()
  })

  it('throws ApiError with code, message and status from the error envelope', async () => {
    server.use(
      http.post(api('/api/tv/shows'), () =>
        HttpResponse.json(
          { error: 'duplicate_item', message: 'You already track this show' },
          { status: 409 },
        ),
      ),
    )

    const err = await request('/api/tv/shows', { method: 'POST', body: { tmdbId: 1399 } }).catch(
      (e: unknown) => e,
    )
    expect(err).toBeInstanceOf(ApiError)
    const apiErr = err as ApiError
    expect(apiErr.status).toBe(409)
    expect(apiErr.code).toBe('duplicate_item')
    expect(apiErr.message).toBe('You already track this show')
  })

  it('falls back to generic code/message on a non-JSON error body', async () => {
    server.use(
      http.get(api('/api/broken'), () => new HttpResponse('Bad Gateway', { status: 502 })),
    )

    const err = await request('/api/broken').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    const apiErr = err as ApiError
    expect(apiErr.status).toBe(502)
    expect(apiErr.code).toBe('unknown_error')
  })

  it('invokes the global unauthorized handler on 401', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    server.use(
      http.get(api('/api/library'), () =>
        HttpResponse.json({ error: 'unauthorized', message: 'Sign in required' }, { status: 401 }),
      ),
    )

    await expect(request('/api/library')).rejects.toMatchObject({ status: 401 })
    expect(handler).toHaveBeenCalledTimes(1)
  })

  it('skips the unauthorized handler when asked (auth/me probe)', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    server.use(
      http.get(api('/api/auth/me'), () =>
        HttpResponse.json({ error: 'unauthorized', message: 'Sign in required' }, { status: 401 }),
      ),
    )

    await expect(request('/api/auth/me', { skipUnauthorizedHandler: true })).rejects.toMatchObject(
      { status: 401 },
    )
    expect(handler).not.toHaveBeenCalled()
  })
})
