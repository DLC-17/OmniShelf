import { act } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { api, server } from '../test/server'
import { renderApp } from '../test/renderApp'

// Shared, hoisted state so the html5-qrcode mock and the tests observe the same
// spies (vi.mock is hoisted above imports).
const camera = vi.hoisted(() => ({
  constructed: 0,
  successCb: null as ((text: string) => void) | null,
  rejectStart: false,
}))

vi.mock('html5-qrcode', () => {
  class Html5Qrcode {
    isScanning = true
    constructor() {
      camera.constructed++
    }
    start(_camera: unknown, _config: unknown, onSuccess: (text: string) => void) {
      camera.successCb = onSuccess
      return camera.rejectStart ? Promise.reject(new Error('denied')) : Promise.resolve()
    }
    stop() {
      return Promise.resolve()
    }
    clear() {}
  }
  return { Html5Qrcode, Html5QrcodeSupportedFormats: { EAN_13: 'EAN_13' } }
})

const me = { id: 1, username: 'david' }

function setSecureContext(value: boolean) {
  Object.defineProperty(window, 'isSecureContext', { value, configurable: true })
}

beforeEach(() => {
  camera.constructed = 0
  camera.successCb = null
  camera.rejectStart = false
  server.use(http.get(api('/api/auth/me'), () => HttpResponse.json(me)))
})

afterEach(() => {
  setSecureContext(false)
})

describe('Scan page — secure-context gate (E6)', () => {
  it('shows the HTTPS banner and never constructs the camera on an insecure context', async () => {
    setSecureContext(false)

    renderApp('/scan')

    const banner = await screen.findByText(/needs a secure \(https\) connection/i)
    expect(banner).toBeInTheDocument()
    expect(screen.getByText(/tailscale https address/i)).toBeInTheDocument()
    // The manual ISBN fallback is always available.
    expect(screen.getByLabelText('ISBN')).toBeInTheDocument()
    // The camera must not be initialized off a secure context.
    expect(camera.constructed).toBe(0)
  })

  it('initializes the camera on a secure context', async () => {
    setSecureContext(true)

    renderApp('/scan')

    await screen.findByTestId('scanner-view')
    await waitFor(() => expect(camera.constructed).toBe(1))
    expect(screen.queryByText(/needs a secure \(https\) connection/i)).not.toBeInTheDocument()
  })
})

describe('Scan page — unknown ISBN (E4)', () => {
  it('drops into manual entry pre-filled with the scanned ISBN after a 404', async () => {
    setSecureContext(true)
    const isbn = '9781234567897'
    server.use(
      http.post(api('/api/books/scan'), () =>
        HttpResponse.json(
          { error: 'book_not_found', message: 'no book found for this ISBN', isbn },
          { status: 404 },
        ),
      ),
    )

    renderApp('/scan')
    await screen.findByTestId('scanner-view')
    await waitFor(() => expect(camera.successCb).not.toBeNull())

    // Simulate the scanner decoding the barcode.
    await act(async () => {
      camera.successCb?.(isbn)
    })

    expect(await screen.findByText(/no book found for isbn/i)).toBeInTheDocument()
    // The manual-entry field is pre-filled with the scanned ISBN.
    expect(await screen.findByLabelText('ISBN')).toHaveValue(isbn)
  })
})

describe('Scan page — camera denial (E7)', () => {
  it('falls back to the manual ISBN form when the camera cannot start', async () => {
    setSecureContext(true)
    camera.rejectStart = true

    renderApp('/scan')

    expect(await screen.findByText(/camera access was blocked/i)).toBeInTheDocument()
    expect(screen.getByLabelText('ISBN')).toBeInTheDocument()
  })
})

describe('Scan page — confirm and track', () => {
  const book = {
    id: 42,
    isbn13: '9780000000002',
    title: 'The Pragmatic Programmer',
    authors: 'Hunt, Thomas',
    coverPath: 'book/9780000000002.jpg',
    pageCount: 352,
  }

  it('tracks a scanned book via the confirm card', async () => {
    setSecureContext(true)
    server.use(
      http.post(api('/api/books/scan'), () => HttpResponse.json(book)),
      http.post(api('/api/books/track'), () =>
        HttpResponse.json(
          {
            id: 7,
            type: 'BOOK',
            externalId: book.isbn13,
            title: book.title,
            status: 'READING',
            progress: 0,
            updatedAt: '2026-07-06T00:00:00Z',
          },
          { status: 201 },
        ),
      ),
    )

    renderApp('/scan')
    await screen.findByTestId('scanner-view')
    await waitFor(() => expect(camera.successCb).not.toBeNull())
    await act(async () => {
      camera.successCb?.(book.isbn13)
    })

    const user = userEvent.setup()
    expect(await screen.findByRole('heading', { name: book.title })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /add to shelf/i }))

    expect(await screen.findByRole('status')).toHaveTextContent(/added to your shelf/i)
  })

  it('handles an already-tracked 409 gracefully', async () => {
    setSecureContext(true)
    server.use(
      http.post(api('/api/books/scan'), () => HttpResponse.json(book)),
      http.post(api('/api/books/track'), () =>
        HttpResponse.json(
          { error: 'already_tracked', message: 'you already track this book' },
          { status: 409 },
        ),
      ),
    )

    renderApp('/scan')
    await screen.findByTestId('scanner-view')
    await waitFor(() => expect(camera.successCb).not.toBeNull())
    await act(async () => {
      camera.successCb?.(book.isbn13)
    })

    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /add to shelf/i }))

    expect(await screen.findByRole('status')).toHaveTextContent(/already track this book/i)
  })
})

describe('Scan page — bulk handheld scanner', () => {
  const book = {
    id: 42,
    isbn13: '9780000000002',
    title: 'The Pragmatic Programmer',
    authors: 'Hunt, Thomas',
    coverPath: 'book/9780000000002.jpg',
    pageCount: 352,
  }

  it('auto-adds each scanned ISBN and keeps the field focused for the next', async () => {
    setSecureContext(false) // works without a camera / secure context
    let tracked = 0
    server.use(
      http.post(api('/api/books/scan'), () => HttpResponse.json(book)),
      http.post(api('/api/books/track'), () => {
        tracked += 1
        return HttpResponse.json(
          { id: 7, type: 'BOOK', externalId: book.isbn13, title: book.title, status: 'READING', progress: 0, updatedAt: '2026-07-06T00:00:00Z' },
          { status: 201 },
        )
      }),
    )

    renderApp('/scan')
    const user = userEvent.setup()

    await user.click(await screen.findByRole('tab', { name: /handheld scanner/i }))
    const field = await screen.findByLabelText('Scan ISBN')

    // A handheld scanner types the ISBN then sends Enter.
    await user.type(field, `${book.isbn13}{Enter}`)

    // Added automatically — no per-book confirmation.
    expect(await screen.findByText(book.title)).toBeInTheDocument()
    expect(await screen.findByText('Added')).toBeInTheDocument()
    expect(tracked).toBe(1)

    // Field cleared and refocused, ready for the next scan.
    await waitFor(() => expect(field).toHaveValue(''))
    expect(field).toHaveFocus()
  })
})
